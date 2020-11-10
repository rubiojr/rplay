package main

import "github.com/blugelabs/bluge"

var bReader *bluge.Reader
var bWriter *bluge.Writer

func blugeReader() *bluge.Reader {
	r, err := bluge.OpenReader(blugeConf)
	if err != nil {
		panic(err)
	}
	return r
}

func blugeWriter() *bluge.Writer {
	var err error
	if bWriter == nil {
		bWriter, err = bluge.OpenWriter(blugeConf)
		if err != nil {
			panic(err)
		}
	}

	return bWriter
}
