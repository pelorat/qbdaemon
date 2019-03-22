package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"qbdaemon/unpacker"
)

func setOverrides(config *config, dest string, temp string) {
	if len(dest) > 0 {
		config.DestPath = dest
	}
	if len(temp) > 0 {
		config.TempPath = temp
	}
}

func main() {

	// Create the default config, this is not valid as it's
	// missing the destination path
	config := newConfig()

	// Setup the command line flags
	configFile := flag.String(
		"config", "./qbd.conf", "configuration file location")

	destPath := flag.String(
		"dest", "", "destination path override")

	tempPath := flag.String(
		"temp", "", "temp directory path override")

	writeConfPath := flag.String(
		"wconf", "", "write default configuration to this location")

	testPath := flag.String("test", "", "a test test")

	var forceWrite bool
	flag.BoolVar(&forceWrite, "force",
		false, "force overwriting when using -wconf")

	flag.Parse()

	if len(*testPath) > 0 {
		up, err := unpacker.New()
		if err != nil {
			log.Fatalln(err)
		}
		files, err := up.ScanPath(context.Background(), *testPath)
		if err != nil {
			log.Fatalln(err)
		}
		for _, path := range files {
			fmt.Println(path)
		}
		os.Exit(0)
	}

	// Handle writing the default config to a file. This
	// function will never return because it calls os.Exit()
	if len(*writeConfPath) > 0 {
		setOverrides(config, *destPath, *tempPath)
		if err := config.writeConfig(*writeConfPath, forceWrite); err != nil {
			fmt.Println("Could not write configuration file: ", err)
			os.Exit(1)
		}
		fmt.Println("Configuration file created:", *writeConfPath)
		os.Exit(0)
	}

	// Load the config, the values are superimposed
	// onto the default configuration structure
	if err := config.loadConfig(*configFile); err != nil {
		log.Fatalln(err)
	}

	setOverrides(config, *destPath, *tempPath)

	// If there's a log path in the config, open the
	// file and set the log output to write to it
	if len(config.LogPath) > 0 {
		f, err := os.OpenFile(config.LogPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
		if err != nil {
			log.Fatal(err)
		}

		defer f.Close()
		log.SetOutput(f)
	}

	// Valdiate the paths in the config
	if err := config.Validate(); err != nil {
		log.Fatalln(err)
	}

	// Signals to catch CTRL-C
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt)

	// Create the unpacker
	up, err := unpacker.New()
	if err != nil {
		log.Fatalln(err)
	}

	// Setup a cancelable context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create and start the API dispatcher
	d := NewDispatcher(config, up)
	go d.Run(ctx)

	// Main loop
	for {
		select {
		case <-ctx.Done():
			d.Done()
			log.Println("Qbdaemon exiting")
			os.Exit(0)

		case why := <-sigs:
			log.Printf("Shutting down (%s)", why.String())
			cancel()

		case err := <-d.Result():
			if err != nil {
				log.Println(err)
			}
			cancel()
		}
	}
}
