package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/acoshift/goreload/internal"
	"github.com/mattn/go-shellwords"
	"github.com/urfave/cli/v2"
)

var (
	startTime  = time.Now()
	logger     = log.New(os.Stdout, "[goreload] ", 0)
	colorGreen = string([]byte{27, 91, 57, 55, 59, 51, 50, 59, 49, 109})
	colorRed   = string([]byte{27, 91, 57, 55, 59, 51, 49, 59, 49, 109})
	colorReset = string([]byte{27, 91, 48, 109})
)

func main() {
	app := cli.NewApp()
	app.Name = "goreload"
	app.Usage = "A live reload utility for Go web applications."
	app.Action = mainAction
	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:    "bin",
			Aliases: []string{"b"},
			Value:   ".goreload",
			Usage:   "name of generated binary file",
		},
		&cli.StringFlag{
			Name:    "path",
			Aliases: []string{"t"},
			Value:   ".",
			Usage:   "Path to watch files from",
		},
		&cli.StringFlag{
			Name:    "build",
			Aliases: []string{"d"},
			Value:   "",
			Usage:   "Path to build files from (defaults to same value as --path)",
		},
		&cli.StringSliceFlag{
			Name:    "excludeDir",
			Aliases: []string{"x"},
			Value:   &cli.StringSlice{},
			Usage:   "Relative directories to exclude",
		},
		&cli.BoolFlag{
			Name:  "all",
			Usage: "reloads whenever any file changes, as opposed to reloading only on .go file change",
		},
		&cli.StringFlag{
			Name:  "buildArgs",
			Usage: "Additional go build arguments",
		},
		&cli.StringFlag{
			Name:  "logPrefix",
			Usage: "Log prefix",
			Value: "goreload",
		},
	}
	app.Commands = []*cli.Command{
		{
			Name:    "run",
			Aliases: []string{"r"},
			Usage:   "Run the goreload",
			Action:  mainAction,
		},
	}

	if err := app.Run(os.Args); err != nil {
		logger.Fatal(err)
	}
}

func mainAction(c *cli.Context) error {
	logger.SetPrefix(fmt.Sprintf("[%s] ", c.String("logPrefix")))

	all := c.Bool("all")
	wd, err := os.Getwd()
	if err != nil {
		logger.Fatal(err)
		return err
	}

	buildArgs, err := shellwords.Parse(c.String("buildArgs"))
	if err != nil {
		return err
	}

	buildPath := c.String("build")
	if buildPath == "" {
		buildPath = c.String("path")
	}
	builder := internal.NewBuilder(buildPath, c.String("bin"), wd, buildArgs)
	runner := internal.NewRunner(filepath.Join(wd, builder.Binary()), c.Args().Slice()...)
	runner.SetWriter(os.Stdout)

	shutdown(runner)

	// build right now
	build(builder, runner)

	// scan for changes
	scanChanges(c.String("path"), c.StringSlice("excludeDir"), all, func() {
		runner.Kill()
		build(builder, runner)
	})

	return nil
}

func build(builder internal.Builder, runner internal.Runner) {
	logger.Println("Building...")

	err := builder.Build()
	if err != nil {
		logger.Printf("%sBuild failed%s\n", colorRed, colorReset)
		fmt.Println(builder.Errors())
	} else {
		logger.Printf("%sBuild finished%s\n", colorGreen, colorReset)
		runner.Run()
	}

	time.Sleep(100 * time.Millisecond)
}

func scanChanges(watchPath string, excludeDirs []string, allFiles bool, cb func()) {
	for {
		filepath.Walk(watchPath, func(path string, info os.FileInfo, err error) error {
			if path == ".git" && info.IsDir() {
				return filepath.SkipDir
			}
			for _, x := range excludeDirs {
				if x == path {
					return filepath.SkipDir
				}
			}

			// ignore hidden files
			if filepath.Base(path)[0] == '.' {
				return nil
			}

			if (allFiles || filepath.Ext(path) == ".go") && info.ModTime().After(startTime) {
				cb()
				startTime = time.Now()
				return errors.New("done")
			}

			return nil
		})
		time.Sleep(500 * time.Millisecond)
	}
}

func shutdown(runner internal.Runner) {
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		s := <-c
		log.Println("Got signal: ", s)
		err := runner.Kill()
		if err != nil {
			log.Print("Error killing: ", err)
		}
		os.Exit(1)
	}()
}
