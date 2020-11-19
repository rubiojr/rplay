package main

import (
	"fmt"
	"strconv"
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
	f := strings.Title(field)
	if field == "_id" {
		f = "ID"
	}
	if field == "year" {
		y, _ := bluge.DecodeNumericFloat64(value)
		printRow(f, strconv.Itoa(int(y)), headerColor)
	} else {
		v := string(value)
		if v == "" {
			v = "unknown"
		}
		printRow(f, v, headerColor)
	}
}
