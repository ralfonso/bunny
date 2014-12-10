package main

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/paulsmith/gogeos/geos"
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

func nearestPark(parks []Park, store Dispensary) (nearest *ParkDistance) {

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

	return nearest
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

	var nearest *ParkDistance

	for _, store := range stores {
		distanceForStore := nearestPark(parks, store)
		if nearest == nil || distanceForStore.distance < nearest.distance {
			nearest = distanceForStore
		}
	}

	printStore(nearest.store)
	fmt.Println(fmt.Sprintf("nearest park: %s, %f", nearest.park.Name, nearest.distance))
}
