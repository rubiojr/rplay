package main

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/hajimehoshi/oto"
	"github.com/jfreymuth/oggvorbis"

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

func playReader(ctx context.Context, t string, reader io.Reader) error {
	rate, d, err := readerFromAudioTypeReader(t, reader)
	if err != nil {
		return err
	}

	c, err := oto.NewContext(rate, 2, 2, 8192)
	if err != nil {
		return err
	}
	defer c.Close()

	player := c.NewPlayer()
	defer player.Close()

	_, err = io.Copy(player, NewReader(ctx, d))
	return err
}

func play(ctx context.Context, t string, b []byte) error {
	rate, d, err := readerFromAudioType(t, b)
	if err != nil {
		return err
	}

	c, err := oto.NewContext(rate, 2, 2, 8192)
	if err != nil {
		return err
	}
	defer c.Close()

	player := c.NewPlayer()
	defer player.Close()

	_, err = io.Copy(player, NewReader(ctx, d))
	return err
}

func readerFromAudioType(t string, b []byte) (int, io.Reader, error) {
	f := bytes.NewReader(b)

	switch t {
	case "ogg":
		d, err := oggvorbis.NewReader(f)
		if err != nil {
			return 0, nil, err
		}
		return d.SampleRate(), NewReaderFromFloat32Reader(d), nil
	case "mp3":
		d, err := mp3.NewDecoder(f)
		if err != nil {
			return 0, nil, err
		}
		return d.SampleRate(), d, nil
	default:
		return 0, nil, fmt.Errorf("unsupported audio type %s", t)
	}
}

func readerFromAudioTypeReader(t string, f io.Reader) (int, io.Reader, error) {
	switch t {
	case "audio/ogg":
		d, err := oggvorbis.NewReader(f)
		if err != nil {
			return 0, nil, err
		}
		return d.SampleRate(), NewReaderFromFloat32Reader(d), nil
	case "audio/mpeg":
		d, err := mp3.NewDecoder(f)
		if err != nil {
			return 0, nil, err
		}
		return d.SampleRate(), d, nil
	default:
		return 0, nil, fmt.Errorf("unsupported audio type %s", t)
	}
}
