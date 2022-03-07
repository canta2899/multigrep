/**
    Multigrep
    =========

    A CLI tool that allows faster recursive search of files
    whose content matches the given pattern.

    It returns the equivalent of grep -E -r -l "pattern" "path".

    Run multigrep --help for a brief usage guide
**/

package main

import (
	"errors"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"sync"
	"syscall"
	"github.com/fatih/color"
)

// One megabyte
const MEGABYTE int64 = 1048576

// Runes for emoji
const OK string = string('\u2713')
const KO string = string('\u00D7')

// Colors for printing
var green =  color.New(color.FgHiGreen).SprintFunc()
var red   =  color.New(color.FgRed).SprintFunc()
var cyan  =  color.New(color.FgCyan).SprintFunc()


// Routine performed by each worker
func handler(ch <-chan string, wg *sync.WaitGroup, r *regexp.Regexp) {
    defer wg.Done()

    for {

        filepath, more := <-ch

        if !more {
            return
        }

        fi, err := os.Stat(filepath)

        if err != nil || !fi.Mode().IsRegular() {
            continue // Skips
        }

        filedata, err := os.ReadFile(filepath)

        if err != nil {
            continue // Skips
        }

        if r.Match(filedata) {
            log.Printf("%v %v\n", green(OK), filepath)
        }
    }
}

// Process path and enqueues if valid for match checking
func processPath(info *os.FileInfo, pathname string, c chan string, excludes []string) error {
    isdir := (*info).IsDir()

    for _, n := range excludes {
        match, _ := filepath.Match(n, pathname)
        match2, _ := filepath.Match(n, filepath.Base(pathname))
        if isdir && (match || match2) {
            return filepath.SkipDir 
        }
    }

    if !isdir && (*info).Size() < MEGABYTE {
        c <- pathname
    }

    return nil
}

func handlePathError(info *os.FileInfo, pathname string) error {
    log.Printf("%v %v\n", red(KO), pathname)

    if (*info).IsDir() {
        return filepath.SkipDir
    } else {
        return nil
    }
}

// Handler for sigterm (ctrl + c from cli)
func setSignalHandlers(closed *bool, wg *sync.WaitGroup) {
    sigch := make(chan os.Signal)
    signal.Notify(sigch, os.Interrupt, syscall.SIGTERM)
    go func() {
        <-sigch 
        *closed = true
    }()
}

func setupLogger() {
    // With flags 0 only the message is printed, without
    // information about time, log level and similar 
    log.SetFlags(0)
}

func main() {
    var wg sync.WaitGroup
    ch := make(chan string, 5000)
    closed := false

    setupLogger()
    setSignalHandlers(&closed, &wg)
    params := ParseArgs()

    color.NoColor = (*params.nocolor)
    pattern := *params.pattern
    if *params.icase {
        pattern = "(?i)" + pattern
    }

    r, _ := regexp.Compile(pattern)

    wg.Add(*params.workers)
    for i := 0; i < *params.workers; i++ {
        go handler(ch, &wg, r)
    }

    filepath.Walk(*params.startpath,

        func(pathname string, info os.FileInfo, err error) error {

            if closed {
                // If the termination is requested, the path Walking
                // stops and the function returns with an error
                return errors.New("User requested termination")
            }

            // Checking permission and access errors
            if err != nil {
                return handlePathError(&info, pathname)
            }

            // Processes path in search of matches with the given
            // pattern or the excluded directories 
            return processPath(&info, pathname, ch, *params.exclude)

        })

    // The channel is closed, this communicates that
    // no more values will be enqueued
    close(ch)

    // Waits for goroutines to finish
    wg.Wait()
}

