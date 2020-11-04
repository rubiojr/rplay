package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/blugelabs/bluge"
	gap "github.com/muesli/go-app-paths"
	"github.com/rubiojr/rapi"
	"github.com/urfave/cli/v2"
)

var appCommands []*cli.Command
var repoPath string
var globalOptions = rapi.DefaultOptions
var blugeConf bluge.Config
var dataDir string
var indexPath string
var firstTimeIndex = false
var exitCh = make(chan os.Signal, 1)

func exist(file string) bool {
	_, err := os.Stat(file)
	if err == nil {
		return true
	}

	return false
}

func isDir(file string) bool {
	fi, err := os.Stat(file)
	if err == nil {
		return true
	}

	return fi.IsDir()
}

func init() {
	scope := gap.NewScope(gap.User, "rplay")
	dirs, err := scope.DataDirs()
	if err != nil {
		panic(err)
	}
	dataDir = dirs[0]
	os.MkdirAll(dataDir, 0755)

	indexPath = filepath.Join(dataDir, "rplay.bluge")
	blugeConf = bluge.DefaultConfig(indexPath)
	if !exist(indexPath) {
		firstTimeIndex = true
	}
	go func() {
		<-exitCh
		os.Exit(0)
	}()
	signal.Notify(exitCh, syscall.SIGINT)
}

func main() {
	var err error
	app := &cli.App{
		Name:     "rapi",
		Commands: []*cli.Command{},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "repo",
				Aliases:     []string{"r"},
				EnvVars:     []string{"RESTIC_REPOSITORY"},
				Usage:       "Repository path",
				Required:    false,
				Destination: &globalOptions.Repo,
			},
			&cli.StringFlag{
				Name:        "password",
				Aliases:     []string{"p"},
				EnvVars:     []string{"RESTIC_PASSWORD"},
				Usage:       "Repository password",
				Required:    false,
				Destination: &globalOptions.Password,
				DefaultText: " ",
			},
			&cli.BoolFlag{
				Name:     "debug",
				Aliases:  []string{"d"},
				Usage:    "Enable debugging",
				Required: false,
			},
		},
	}

	app.Commands = append(app.Commands, appCommands...)
	err = app.Run(os.Args)
	if err != nil {
		println(fmt.Sprintf("\n🛑 %s", err))
	}
}

func needsIndex() bool {
	return firstTimeIndex
}
