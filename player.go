package main

import (
	"bytes"
	"context"
	"io"

	"github.com/hajimehoshi/oto"

	"github.com/hajimehoshi/go-mp3"
)

type readerCtx struct {
	ctx context.Context
	r   io.Reader
}

func (r *readerCtx) Read(p []byte) (n int, err error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

func NewReader(ctx context.Context, r io.Reader) io.Reader {
	return &readerCtx{
		ctx: ctx,
		r:   r,
	}
}

func play(ctx context.Context, b []byte) error {
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

	player := c.NewPlayer()
	defer player.Close()

	_, err = io.Copy(player, NewReader(ctx, d))
	return err
}
