package main

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/paulsmith/gogeos/geos"
)

const (
	workers int = 30
)

// convenience types for arrays
type Dispensaries []Dispensary
type Parks []Park

type DispensaryKml struct {
	XMLName      xml.Name     `xml:"kml"`
	Dispensaries Dispensaries `xml:"Document>Folder>Placemark"`
}

type ParkKml struct {
	XMLName xml.Name `xml:"kml"`
	Parks   Parks    `xml:"Document>Folder>Placemark"`
}

// base type for KML Placemark objects
type Placemark struct {
	Name        string `xml:"name"`
	Description string `xml:"description"`
	Address     string `xml:"address"`
	Geometry    *geos.Geometry
	rating      int
}

// Dispensary type using the Placemark as a mixin
// with point coordinates
type Dispensary struct {
	Placemark
	PointCoords string `xml:"Point>coordinates"`
}

// Park type using the Placemark as a mixin
// with polygon coordinates
type Park struct {
	Placemark
	LinearRingCoords string `xml:"MultiGeometry>Polygon>outerBoundaryIs>LinearRing>coordinates"`
}

// custom XML unmarshal function for Dispensary placemarks
func (dispensaries *Dispensaries) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	dispensary := &Dispensary{}

	err := d.DecodeElement(dispensary, &start)
	if err != nil {
		return nil
	}

	coords := transformCoordinates(dispensary.PointCoords)
	if coords == nil {
		return nil
	}

	dispensary.Geometry, err = geos.NewPoint(coords[0])
	if err != nil {
		return nil
	}

	newSlice := []Dispensary(*dispensaries)
	*dispensaries = append(newSlice, *dispensary)

	return nil
}

// custom XML unmarshal function for Park placemarks
func (parks *Parks) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	park := &Park{}

	err := d.DecodeElement(park, &start)
	if err != nil {
		return nil
	}

	coords := transformCoordinates(park.LinearRingCoords)

	park.Geometry, err = geos.NewPolygon(coords)
	if err != nil {
		return nil
	}

	newSlice := []Park(*parks)
	*parks = append(newSlice, *park)

	return nil
}

// takes a string of polygon coordinates and convert
// the pairs first to floats and then to an array
// of geos.Coord objects
func transformCoordinates(coordinates string) (geosCoords []geos.Coord) {
	trimmed := strings.TrimSpace(coordinates)
	coordGroups := strings.Split(trimmed, " ")
	coordPairs := make([][]string, len(coordGroups))

	for idx, raw := range coordGroups {
		trimmedGroup := strings.TrimRight(raw, ",0")
		pair := strings.Split(trimmedGroup, ",")
		coordPairs[idx] = pair
	}

	for _, pair := range coordPairs {
		x, err := strconv.ParseFloat(pair[0], 64)
		if err != nil {
			return nil
		}

		y, err := strconv.ParseFloat(pair[1], 64)
		if err != nil {
			return nil
		}

		geosCoords = append(geosCoords, geos.NewCoord(x, y))
	}

	return geosCoords
}

// takes a geos.Geometry object and returns its lat/lng (X/Y)
// values, dangerously ignoring errors
func extractLatLng(geometry *geos.Geometry) (lat, lng float64) {
	lng, _ = geometry.X()
	lat, _ = geometry.Y()
	return lat, lng
}

// an example using the empty interface to take any object.
// hands of to Go's XML library for unmarshalling
func kmlToPlacemarks(kmlFileName string, intf interface{}) {
	// declare some vars
	var kmlFile []byte
	var err error

	// read the dispensaries file into the byte array
	kmlFile, err = ioutil.ReadFile(kmlFileName)

	// common error checking pattern in Go
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// read the byte array into the struct instance.
	err = xml.Unmarshal(kmlFile, intf)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// a type to contain a park and its distance
// from a dispensary
type ParkDistance struct {
	store    Dispensary
	park     Park
	distance float64
}

func nearestParks(wg *sync.WaitGroup, parks []Park, input <-chan Dispensary, output chan<- *ParkDistance) {

	// defer a function to signal the waitgroup that this worker is complete
	// runs regardless of errors/panic
	defer wg.Done()

	// range is channel aware, will loop and block
	// until the channel is closed
	for store := range input {
		// use a pointer for the convenience of nil
		var nearest *ParkDistance

		for _, park := range parks {
			// artificial latency!
			time.Sleep(100 * time.Microsecond)
			distance, err := store.Geometry.Distance(park.Geometry)

			if err == nil {
				if nearest == nil || distance < nearest.distance {
					nearest = &ParkDistance{store: store, park: park, distance: distance}
				}
			} else {
				fmt.Println(err)
			}
		}

		// push the result onto the output channel
		output <- nearest
	}
}

// a worker that reads from a result channel and does some tracking to
// find the shortest distance among them
func nearestPair(wg *sync.WaitGroup, results <-chan *ParkDistance) chan *ParkDistance {

	result := make(chan *ParkDistance)

	go func() {
		var nearest *ParkDistance

		// range will block until the channel is exhausted AND closed
		for parkDistance := range results {
			if nearest == nil || parkDistance.distance < nearest.distance {
				nearest = parkDistance
			}
		}

		result <- nearest
	}()

	return result
}

func sanitizeAddress(address string) (clean string) {
	whitespaceRe := regexp.MustCompile("\\s{2,}")
	// an example where optional arguments would be nice
	clean = strings.Replace(address, "\n", "", -1)
	// clean up spaces
	clean = whitespaceRe.ReplaceAllLiteralString(clean, " ")
	// optional return value (don't do this)
	return
}

func printStore(store Dispensary) {
	cleanAddress := sanitizeAddress(store.Address)
	mapsUrl := "https://maps.google.com/maps/?q=%f,%f&z=17"
	lat, lng := extractLatLng(store.Geometry)
	mapUrl := fmt.Sprintf(mapsUrl, lat, lng)
	fmt.Println(fmt.Sprintf("%s\n%s\n%s", store.Name, cleanAddress, mapUrl))
}

func main() {
	var storeKml DispensaryKml
	var parkKml ParkKml

	kmlToPlacemarks("assets/dispensaries.kml", &storeKml)
	kmlToPlacemarks("assets/parks.kml", &parkKml)

	stores := storeKml.Dispensaries
	parks := parkKml.Parks

	// create channels for the work queue and the results
	// these channels are buffered to avoid blocking writes
	// we have a small data set, so just use that buffer size
	storeCount := len(stores)
	workQueue := make(chan Dispensary, storeCount)
	results := make(chan *ParkDistance, storeCount)

	// create a wait group to track worker completion
	var wg sync.WaitGroup

	// start up our workers, incrementing the WaitGroup each time
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go nearestParks(&wg, parks, workQueue, results)
	}

	finalResult := nearestPair(&wg, results)

	// all workers are waiting for input

	// queue up all of the stores for distance checks
	for _, store := range stores {
		workQueue <- store
	}

	// once we've queued all stores, we close the channel so
	// the workers can exit
	close(workQueue)

	// if we don't wait, we have a race condition. make sure
	// all workers finish their jobs and exit
	wg.Wait()

	// all the workers have completed, so we can close the results
	// channel, which tells the collector it can exit when it's done
	close(results)

	// blocks until the channel has data.
	nearest := <-finalResult

	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("Nearest To A Park")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println()

	printStore(nearest.store)
	fmt.Println(fmt.Sprintf("nearest park: %s, %f", nearest.park.Name, nearest.distance))
}
