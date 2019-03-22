package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	qbc "qbdaemon/qbclient"
	"qbdaemon/unpacker"
	"sync"
	"time"
)

// SetCategory ...
type SetCategory struct {
	hash     string
	category string
}

// AddCategory ...
type AddCategory struct {
	category string
}

// GetTorrents ...
type GetTorrents struct{}

// DeQueue ...
type DeQueue struct {
	hash string
}

// Dispatcher ...
type Dispatcher struct {
	wg       *sync.WaitGroup
	cfg      *config
	up       *unpacker.Unpacker
	result   chan error
	timer    *time.Timer
	timeouts int
	actions  chan interface{}
	done     chan interface{}
	tm       *TorrentQueue
}

// NewDispatcher ...
func NewDispatcher(cfg *config, up *unpacker.Unpacker) *Dispatcher {
	return &Dispatcher{
		wg:      &sync.WaitGroup{},
		cfg:     cfg,
		up:      up,
		result:  make(chan error),
		actions: make(chan interface{}, 100),
		tm:      NewTorrentQueue(cfg),
		done:    make(chan interface{}),
	}
}

// Result ...
func (d *Dispatcher) Result() <-chan error {
	return d.result
}

// Done waits for Run() to finish
func (d *Dispatcher) Done() {
	<-d.done
}

func (d *Dispatcher) logTimeout(err error) {
	d.timeouts++
	if d.timeouts < 4 {
		log.Println(err)
	}
}

func (d *Dispatcher) resetTimeout() {
	d.timeouts = 0
}

func (d *Dispatcher) resetTimer() {
	if d.timer == nil {
		d.timer = time.AfterFunc(
			time.Duration(d.cfg.Polling.Delay)*time.Second,
			func() { d.actions <- GetTorrents{} })
	} else {
		d.timer.Reset(time.Duration(d.cfg.Polling.Delay) * time.Second)
	}

}
func (d *Dispatcher) stopTimer() {
	if d.timer != nil {
		d.timer.Stop()
	}
}

func (d *Dispatcher) closeActionChan() {
	close(d.actions)
}

func (d *Dispatcher) closeResultChan() {
	close(d.result)
}

func (d *Dispatcher) signalDone() {
	close(d.done)
}

func (d *Dispatcher) waitGroupEnter() {
	d.wg.Add(1)
}

func (d *Dispatcher) waitGroupLeave() {
	d.wg.Done()
}

// QueueAction ...
func (d *Dispatcher) QueueAction(a interface{}) {
	d.actions <- a
}

func (d *Dispatcher) workerUnpack(ctx context.Context, w uint, jobs <-chan TorrentJob) {
	d.waitGroupEnter()
	defer d.waitGroupLeave()

	for {
		select {
		case job := <-jobs:

			torrent := job.GetTorrent()
			scanPath := filepath.Join(torrent.SavePath, torrent.Name)
			log.Printf("[Unpack/%d] Unpacking %s (%s)", w, torrent.Hash, torrent.Name)

			// Scan the path for targets to unpack
			targets, err := d.up.ScanPath(ctx, scanPath)
			if err == context.Canceled {
				// When canceled it means we just exit because we're shutting down
				return
			} else if err != nil {
				// Some other error occurred, log the issue and set the category to error
				log.Printf("[Unpack/%d] Error scanning path for torrent %s (%s); %s",
					w, torrent.Hash, scanPath, err.Error())

				d.actions <- SetCategory{
					hash:     torrent.Hash,
					category: d.cfg.Categories.Error,
				}

			} else {
				if len(targets) == 0 {
					// No targets; set the category to NoArchive
					d.actions <- SetCategory{
						hash:     torrent.Hash,
						category: d.cfg.Categories.NoArchive,
					}
				} else {
					// We have targets to unpack, open a log file
					unpackError := false
					destPath := filepath.Join(d.cfg.DestPath, torrent.Name)
					os.MkdirAll(destPath, os.ModePerm)
					logPath := filepath.Join(destPath, "unpack.log")
					logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
					if err != nil {
						// Some other error occurred, log the issue and set the category to error
						log.Printf("[Unpack/%d] Error opening logfile '%s' path for torrent %s (%s); %s",
							w, logPath, torrent.Hash, torrent.Name, err.Error())

						d.actions <- SetCategory{
							hash:     torrent.Hash,
							category: d.cfg.Categories.Error,
						}
					} else {

						d.actions <- SetCategory{
							hash:     torrent.Hash,
							category: d.cfg.Categories.UnpackBusy,
						}

						for _, target := range targets {
							err = target.Unpack(ctx, destPath+string(filepath.Separator), logFile)
							if err == context.Canceled {
								// When canceled it means we just exit because we're shutting down
								logFile.Close()
								return
							}

							if err != nil {
								unpackError = true
								log.Printf("[Unpack/%d] Error unpacking target %s; %s",
									w, target.String(), err.Error())
							}
						}
						logFile.Close()
					}

					if err == nil && !unpackError {
						d.actions <- SetCategory{
							hash:     torrent.Hash,
							category: d.cfg.Categories.UnpackDone,
						}
					} else {
						d.actions <- SetCategory{
							hash:     torrent.Hash,
							category: d.cfg.Categories.Error,
						}
					}
				}
			}

			// Remove the queued status from this torrent
			d.tm.JobDone(torrent.Hash)
		case <-ctx.Done():
			return
		}
	}
}

func (d *Dispatcher) workerCheck(ctx context.Context, w uint, jobs <-chan TorrentJob) {
	d.waitGroupEnter()
	defer d.waitGroupLeave()

	for {
		select {
		case job := <-jobs:
			torrent := job.GetTorrent()
			scanPath := filepath.Join(torrent.SavePath, torrent.Name)
			log.Printf("[Check/%d] Checking %s (%s) for archives", w, torrent.Hash, scanPath)

			// Scan the path for targets
			targets, err := d.up.ScanPath(ctx, scanPath)
			if err == context.Canceled {
				// When canceled it means we just exit because we're shutting down
				return
			} else if err != nil {
				// Some other error occurred, log the issue and set the category to error
				log.Printf("[Check/%d] Error scanning path for torrent %s (%s); %s",
					w, torrent.Hash, torrent.Name, err.Error())

				d.actions <- SetCategory{
					hash:     torrent.Hash,
					category: d.cfg.Categories.Error,
				}
			} else {
				if len(targets) == 0 {
					d.actions <- SetCategory{
						hash:     torrent.Hash,
						category: d.cfg.Categories.NoArchive,
					}
				} else {
					d.actions <- SetCategory{
						hash:     torrent.Hash,
						category: d.cfg.Categories.Default,
					}
				}
			}
			d.tm.JobDone(torrent.Hash)
		case <-ctx.Done():
			return
		}
	}
}

// Run ...
func (d Dispatcher) Run(ctx context.Context) {

	// Setup
	defer d.signalDone()
	defer d.closeActionChan()
	defer d.closeResultChan()
	defer d.stopTimer()

	// Create a new torrent API client
	tc := qbc.NewClient(d.cfg.Server, d.cfg.Port, d.cfg.Username, d.cfg.Password)

	// Setup logging callbacks
	d.tm.setAddEvent(func(t *qbc.Torrent) {
		log.Printf("[Queue] Added torrent %s (%s)\n", t.Hash, t.Name)
	})
	d.tm.setRemoveEvent(func(t *qbc.Torrent) {
		log.Printf("[Queue] Removed torrent %s (%s)\n", t.Hash, t.Name)
	})
	d.tm.setQueueFullEvent(func(t *qbc.Torrent, j TorrentJob) {
		log.Printf("[Queue] Job queue full; failed to enqueue torrent %s (%s) for job %T\n", t.Hash, t.Name, j)
	})

	// Start the torrent unpack workers
	for i := uint(0); i < d.cfg.Workers.Unpack; i++ {
		go d.workerUnpack(ctx, i, d.tm.QueueA())
	}

	// Start the torrent check workers
	for i := uint(0); i < d.cfg.Workers.Check; i++ {
		go d.workerCheck(ctx, i, d.tm.QueueB())
	}

	// Buffer the initial actions
	d.actions <- AddCategory{category: d.cfg.Categories.Default}
	d.actions <- AddCategory{category: d.cfg.Categories.Error}
	d.actions <- AddCategory{category: d.cfg.Categories.NoArchive}
	d.actions <- AddCategory{category: d.cfg.Categories.UnpackBusy}
	d.actions <- AddCategory{category: d.cfg.Categories.UnpackDone}
	d.actions <- AddCategory{category: d.cfg.Categories.UnpackStart}
	d.actions <- GetTorrents{}

	log.Printf("[Manager] Up and running with %d workers", d.cfg.Workers.Check+d.cfg.Workers.Unpack)

	// Main loop
	loop := true
	for {
		var err error
		select {
		case actionType := <-d.actions:
			// retry loop
			for {
				ctx, cancel := context.WithTimeout(ctx, time.Duration(d.cfg.Polling.Timeout)*time.Second)

				switch actionType.(type) {
				case GetTorrents:
					torrents, err := tc.GetTorrents(ctx, nil)
					if err == nil {
						d.tm.Update(torrents)
						d.resetTimer()
					}

				case AddCategory:
					action, _ := actionType.(AddCategory)
					err = tc.AddCategory(ctx, action.category)
					switch err {
					case qbc.ErrCategoryBad:
						log.Printf("[Manager] Failed to add category %s (does it already exist?)\n", action.category)
						err = nil
					case nil:
						log.Println("[Manager] Added category", action.category, "to torrent client")
					}

				case SetCategory:
					action, _ := actionType.(SetCategory)
					err = tc.SetCategory(ctx, action.hash, action.category)
					if err == nil {
						log.Printf("[Manager] Category for torrent %s changed to %s\n", action.hash, action.category)
					}

				case DeQueue:
					action, _ := actionType.(DeQueue)
					d.tm.JobDone(action.hash)
				}

				cancel()

				if err == context.DeadlineExceeded {
					d.logTimeout(err)
					continue
				}

				break
			}

			if err != nil {
				switch err {
				case qbc.ErrLogin:
					fallthrough
				case qbc.ErrBanned:
					fallthrough
				case qbc.ErrCategoryEmpty:
					fallthrough
				case qbc.ErrCategoryBad:
					fallthrough
				default:
					fallthrough
				case context.Canceled:
					d.result <- err
					loop = false
				}
			}

		case <-ctx.Done():
			loop = false
		}

		if !loop {
			break
		}
	}

	d.wg.Wait()
	log.Println("[Manager] Shutting down")
}
