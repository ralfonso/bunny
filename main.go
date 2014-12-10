package main

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strings"
)

// convenience types for arrays
type Placemarks []Placemark

type DispensaryKml struct {
	XMLName      xml.Name   `xml:"kml"`
	Dispensaries Placemarks `xml:"Document>Folder>Placemark"`
}

// base type for KML Placemark objects
type Placemark struct {
	Name        string `xml:"name"`
	Description string `xml:"description"`
	Address     string `xml:"address"`
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

func sanitizeAddress(address string) (clean string) {
	whitespaceRe := regexp.MustCompile("\\s{2,}")
	// an example where optional arguments would be nice
	clean = strings.Replace(address, "\n", "", -1)
	// clean up spaces
	clean = whitespaceRe.ReplaceAllLiteralString(clean, " ")
	// optional return value (don't do this)
	return
}

func printStore(store Placemark) {
	cleanAddress := sanitizeAddress(store.Address)
	fmt.Println(fmt.Sprintf("%s\n%s", store.Name, cleanAddress))
}

func main() {
	var storeKml DispensaryKml
	kmlToPlacemarks("assets/dispensaries.kml", &storeKml)
	stores := storeKml.Dispensaries

	for idx, store := range stores {
		fmt.Print("#", idx, ": ")
		printStore(store)

		if idx < len(stores)-1 {
			fmt.Println()
		}
	}
}
