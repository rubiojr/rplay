package main

import "errors"

var errNeedsIndex = errors.New("rplay index does not exist. Use 'rplay index' to create it first")
