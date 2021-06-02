package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

type Graph struct {
	Links []struct {
		Source int `json:"source"`
		Target int `json:"target"`
	} `json:"links"`
}

var filename = flag.String("filename", "", "the .json input file to convert")

func main() {
	flag.Parse()
	if filename == nil || *filename == "" {
		flag.PrintDefaults()
		return
	}
	f, err := os.Open(*filename)
	if err != nil {
		panic(err)
	}
	d := json.NewDecoder(f)
	var g Graph
	if err := d.Decode(&g); err != nil {
		panic(err)
	}
	for _, link := range g.Links {
		fmt.Printf("%d %d\n", link.Source, link.Target)
	}
}
