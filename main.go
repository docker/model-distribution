package main

import (
	"flag"
	"log"
	"os"
)

func main() {
	var (
		source = flag.String("source", "", "Path to local file or URL to download")
		tag    = flag.String("tag", "", "Target registry/repository:tag")
	)
	flag.Parse()

	if *source == "" || *tag == "" {
		flag.Usage()
		os.Exit(1)
	}

	_, err := Push(*source, *tag)
	if err != nil {
		log.Fatal(err)
	}
}
