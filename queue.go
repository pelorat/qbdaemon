package main

import (
	qbc "qbdaemon/qbclient"
	"sync"
	"time"
)

type queueStatus int

const (
	tsDefault queueStatus = 0
	tsQueued  queueStatus = 1
	tsRemoved queueStatus = 2
)

type mapItem struct {
	torrent *qbc.Torrent
	status  queueStatus
	time    time.Time
}

// TorrentQueue handles the queuing of torrent jobs
type TorrentQueue struct {
	data      map[string]*mapItem
	mutex     sync.Mutex
	config    *config
	added     func(*qbc.Torrent)
	updated   func(*qbc.Torrent)
	removed   func(*qbc.Torrent)
	queuefull func(*qbc.Torrent, TorrentJob)
	queueA    chan TorrentJob
	queueB    chan TorrentJob
}

// TorrentJob interface declaration
type TorrentJob interface {
	GetTorrent() *qbc.Torrent
}

// CheckTorrent is a job type that checks a torrent for unpackable archives
type CheckTorrent struct {
	torrent *qbc.Torrent
}

// UnpackTorrent is a job type that tries to unpack torrent archives
type UnpackTorrent struct {
	torrent *qbc.Torrent
}

func (mi *mapItem) IsQueued() bool {
	return mi.status == tsQueued
}

// GetTorrent returns the enclosed torrent pointer
func (j *CheckTorrent) GetTorrent() *qbc.Torrent {
	return j.torrent
}

// GetTorrent returns the enclosed torrent pointer
func (j *UnpackTorrent) GetTorrent() *qbc.Torrent {
	return j.torrent
}

func (tm *TorrentQueue) setAddEvent(cb func(*qbc.Torrent)) {
	tm.added = cb
}

func (tm *TorrentQueue) setUpdateEvent(cb func(*qbc.Torrent)) {
	tm.updated = cb
}

func (tm *TorrentQueue) setRemoveEvent(cb func(*qbc.Torrent)) {
	tm.removed = cb
}

func (tm *TorrentQueue) setQueueFullEvent(cb func(*qbc.Torrent, TorrentJob)) {
	tm.queuefull = cb
}

// NewTorrentQueue creates a new concurrent map to hold a torrent list
func NewTorrentQueue(cfg *config) *TorrentQueue {
	return &TorrentQueue{
		data:   make(map[string]*mapItem),
		queueA: make(chan TorrentJob, 100),
		queueB: make(chan TorrentJob, 100),
		mutex:  sync.Mutex{},
		config: cfg,
	}
}

// Lock the TorrentQueue mutex
func (tm *TorrentQueue) Lock() {
	tm.mutex.Lock()
}

// Unlock the TorrentQueue mutex
func (tm *TorrentQueue) Unlock() {
	tm.mutex.Unlock()
}

// Remove a torrent mapping given a hash
func (tm *TorrentQueue) Remove(hash string) {
	tm.Lock()
	defer tm.Unlock()
	delete(tm.data, hash)
}

// Get torrent data given a hash
func (tm *TorrentQueue) Get(hash string) *qbc.Torrent {
	tm.Lock()
	defer tm.Unlock()
	if t, ok := tm.data[hash]; ok {
		return t.torrent
	}
	return nil
}

// JobDone sets the queue status flag back to default
func (tm *TorrentQueue) JobDone(hash string) {
	tm.Lock()
	defer tm.Unlock()
	if t, ok := tm.data[hash]; ok {
		t.status = tsDefault
	}
}

// QueueA returns a queue of torrent jobs
func (tm *TorrentQueue) QueueA() <-chan TorrentJob {
	return tm.queueA
}

// QueueB returns a queue of torrent jobs
func (tm *TorrentQueue) QueueB() <-chan TorrentJob {
	return tm.queueB
}

func (tm *TorrentQueue) enqeueJobs() {
	for _, mi := range tm.data {

		if mi.torrent.IsCompleted() && !mi.torrent.HasCategory() && !mi.IsQueued() {

			// When the torrent is done, is not already in the queue
			// and has no category, then queue it for a check
			// TODO: Add UNIX time comparison to detect torrent age
			torrentJob := &CheckTorrent{torrent: mi.torrent}

			select {
			case tm.queueB <- torrentJob:
				mi.status = tsQueued
				continue
			default:
				// QueueA is full
				if tm.queuefull != nil {
					tm.queuefull(mi.torrent, torrentJob)
				}
			}

		} else if mi.torrent.IsCompleted() && !mi.IsQueued() &&
			mi.torrent.Category == tm.config.Categories.UnpackStart {

			// When the torrent is done, is not already in the queue
			// and the category is set to "Unpack", queue the torrent
			// for an unpacking job

			torrentJob := &UnpackTorrent{torrent: mi.torrent}

			select {
			case tm.queueA <- torrentJob:
				mi.status = tsQueued
				continue
			default:
				// QueueB is full
				if tm.queuefull != nil {
					tm.queuefull(mi.torrent, torrentJob)
				}
			}
		}
	}
}

// Update the torrent queue with a new torrent list
func (tm *TorrentQueue) Update(torrents []*qbc.Torrent) {
	tm.Lock()
	defer tm.Unlock()

	// Check for new and updated torrents
	now := time.Now()
	for _, t := range torrents {
		if meta, ok := tm.data[t.Hash]; ok {
			// Existing torrent
			meta.torrent = t
			meta.time = now

			// Callback
			if tm.updated != nil {
				tm.updated(t)
			}

		} else {
			// New torrent
			tm.data[t.Hash] = &mapItem{
				torrent: t,
				status:  tsDefault,
				time:    now,
			}

			// Callback
			if tm.added != nil {
				tm.added(t)
			}
		}
	}

	// Check for removed torrents
	for k, d := range tm.data {
		if d.time.Before(now) {
			d.status = tsRemoved

			// TODO: this should possibly be removed since communication
			// TODO: with the queue is done via torrent hash and not the
			// TODO: torrent item pointer.
			if time.Since(now) > 60*time.Minute {
				delete(tm.data, k)
			}

			if tm.removed != nil {
				tm.removed(d.torrent)
			}
		}
	}

	// Check for and enqueue the wanted torrent jobs
	tm.enqeueJobs()
}
