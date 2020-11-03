package main

import (
	"bytes"
	"fmt"
	"io"

	"github.com/hajimehoshi/oto"

	"github.com/hajimehoshi/go-mp3"
)

func play(b []byte) error {
	f := bytes.NewReader(b)

	d, err := mp3.NewDecoder(f)
	if err != nil {
		return err
	}

	c, err := oto.NewContext(d.SampleRate(), 2, 2, 8192)
	if err != nil {
		return err
	}
	defer c.Close()

	p := c.NewPlayer()
	defer p.Close()

	fmt.Printf("Length: %d[bytes]\n", d.Length())

	if _, err := io.Copy(p, d); err != nil {
		return err
	}
	return nil
}
