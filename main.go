package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/blugelabs/bluge"
	gap "github.com/muesli/go-app-paths"
	"github.com/rubiojr/rapi"
	"github.com/urfave/cli/v2"
)

var appCommands []*cli.Command
var globalOptions = rapi.DefaultOptions
var blugeConf bluge.Config
var indexPath = defaultIndexPath()

func initApp() {
	os.MkdirAll(defaultIndexDir(), 0755)
	blugeConf = bluge.DefaultConfig(indexPath)
}

func defaultIndexDir() string {
	scope := gap.NewScope(gap.User, "rplay")
	dirs, err := scope.DataDirs()
	if err != nil {
		panic(err)
	}
	return dirs[0]
}

func defaultIndexPath() string {
	return filepath.Join(defaultIndexDir(), "rplay.bluge")
}

func main() {
	var err error
	app := &cli.App{
		Name:     "rplay",
		Commands: []*cli.Command{},
		Version:  "v0.3.1",
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
			&cli.StringFlag{
				Name:        "index-path",
				Usage:       "Index path",
				Required:    false,
				Destination: &indexPath,
				Value:       defaultIndexPath(),
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
