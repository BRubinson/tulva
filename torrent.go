// Copyright 2013 Jari Takkala. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"launchpad.net/tomb"
	"log"
)

type Torrent struct {
	metaInfo MetaInfo
	infoHash []byte
	peer chan PeerTuple
	Stats Stats
	t tomb.Tomb
}

type Stats struct {
	Left int
	Uploaded int
	Downloaded int
}

// Metainfo File Structure
type MetaInfo struct {
	Info struct {
		PieceLength int "piece length"
		Pieces      string
		Private     int
		Name        string
		Length      int
		Md5sum      string
		Files []struct {
			Length int
			Md5sum string
			Path   []string
		}
	}
	Announce     string
	AnnounceList [][]string "announce-list"
	CreationDate int        "creation date"
	Comment      string
	CreatedBy    string "created by"
	Encoding     string
}

// Init completes the initalization of the Torrent structure
func (t *Torrent) Init() {
	// Initialize bytes left to download
	if len(t.metaInfo.Info.Files) > 0 {
		for _, file := range(t.metaInfo.Info.Files) {
			t.Stats.Left += file.Length
		}
	} else {
		t.Stats.Left = t.metaInfo.Info.Length
	}
	// TODO: Read in the file and adjust bytes left
}

func (t *Torrent) Stop() error {
	t.t.Kill(nil)
	return t.t.Wait()
}

// Run starts the Torrent session and orchestrates all the child processes
func (t *Torrent) Run() {
	log.Println("Torrent : Run : Started")
	defer t.t.Done()
	defer log.Println("Torrent : Run : Completed")
	t.Init()

	completedCh := make(chan bool)
	peersCh := make(chan PeerTuple)
	statsCh := make(chan Stats)

	io := new(IO)
	io.metaInfo = t.metaInfo
	go io.Run()

	trackerManager := new(TrackerManager)
	trackerManager.peersCh = peersCh
	trackerManager.completedCh = completedCh
	trackerManager.statsCh = statsCh
//	go trackerManager.Run(t.metaInfo, t.infoHash)

	peerManager := new(PeerManager)
	peerManager.peersCh = peersCh
	peerManager.statsCh = statsCh
	go peerManager.Run()

	for {
		select {
		case <- t.t.Dying():
			trackerManager.Stop()
			return
		}
	}
}

