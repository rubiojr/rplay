package main

import (
	"fmt"
	"strings"

	"github.com/blugelabs/bluge"
	"github.com/muesli/reflow/padding"
	"github.com/muesli/termenv"
)

const (
	headerColor = "#ffb236"
	colPadding  = 20
)

var filterField = func(name string) bool {
	switch name {
	case "_id", "album", "genre", "year", "filename", "title", "artist":
		return false
	default:
		return true
	}
}

func colorize(str, color string) string {
	out := termenv.String(str)
	p := termenv.ColorProfile()
	return out.Foreground(p.Color(color)).String()
}

func printRow(header, value, color string) {
	fmt.Printf("%s %s\n", padding.String(colorize(header+":", color), colPadding), value)
}

func printMetadata(field string, value []byte, color string) {
	var f string
	if field == "_id" {
		f = "ID"
	} else if field == "repository_id" {
		f = "Repository ID"
	} else {
		f = strings.Title(strings.ReplaceAll(field, "_", " "))
	}

	var v string
	switch field {
	case "mtime":
		t, err := bluge.DecodeDateTime(value)
		if err != nil {
			v = "error"
		} else {
			v = fmt.Sprintf(t.Format("2006-1-2"))
		}
	case "size":
		t, err := bluge.DecodeNumericFloat64(value)
		if err != nil {
			v = "error"
		} else {
			v = fmt.Sprintf("%0.f", t)
		}
	case "year":
		y, err := bluge.DecodeNumericFloat64(value)
		if err != nil {
			v = "error"
		}
		v = fmt.Sprintf("%0.f", y)
	default:
		v = string(value)
	}

	printRow(f, v, headerColor)
}
