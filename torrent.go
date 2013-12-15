// Copyright 2013 Jari Takkala. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"code.google.com/p/bencode-go"
	"crypto/sha1"
	"errors"
	"launchpad.net/tomb"
	"log"
	"os"
)

type Torrent struct {
	metaInfo MetaInfo
	infoHash []byte
	peer     chan PeerTuple
	Stats    Stats
	t        tomb.Tomb
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
		Files       []struct {
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

// ParseTorrentFile opens the torrent filename specified and parses it,
// returning a Torrent structure with the MetaInfo and SHA-1 hash of the
// Info dictionary.
func ParseTorrentFile(filename string) (torrent Torrent, err error) {
	file, err := os.Open(filename)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	// Decode the file into a generic bencode representation
	m, err := bencode.Decode(file)
	if err != nil {
		return
	}
	// WTF?: Understand the next line
	metaMap, ok := m.(map[string]interface{})
	if !ok {
		err = errors.New("Couldn't parse torrent file")
		return
	}
	infoDict, ok := metaMap["info"]
	if !ok {
		err = errors.New("Unable to locate info dict in torrent file")
		return
	}

	// Create an Info dict based on the decoded file
	var b bytes.Buffer
	err = bencode.Marshal(&b, infoDict)
	if err != nil {
		return
	}

	// Compute the info hash
	h := sha1.New()
	h.Write(b.Bytes())
	torrent.infoHash = append(torrent.infoHash, h.Sum(nil)...)

	// Populate the metaInfo structure
	file.Seek(0, 0)
	err = bencode.Unmarshal(file, &torrent.metaInfo)
	if err != nil {
		return
	}

	log.Printf("Parse : ParseTorrentFile : Successfully parsed %s", filename)
	log.Printf("Parse : ParseTorrentFile : The length of each piece is %d", torrent.metaInfo.Info.PieceLength)

	return
}

// Init completes the initalization of the Torrent structure
func (t *Torrent) Init() {
	// Initialize bytes left to download
	if len(t.metaInfo.Info.Files) > 0 {
		for _, file := range t.metaInfo.Info.Files {
			t.Stats.Left += file.Length
		}
	} else {
		t.Stats.Left = t.metaInfo.Info.Length
	}
	// TODO: Read in the file and adjust bytes left
}

// Stop stops this Torrent session
func (t *Torrent) Stop() error {
	log.Println("Torrent : Stop : Stopping")
	t.t.Kill(nil)
	return t.t.Wait()
}

// Run starts the Torrent session and orchestrates all the child processes
func (t *Torrent) Run() {
	log.Println("Torrent : Run : Started")
	defer t.t.Done()
	defer log.Println("Torrent : Run : Completed")
	t.Init()

	pieceHashes := make([][]byte, 0)
	for offset := 0; offset <= len(t.metaInfo.Info.Pieces)-20; offset += 20 {
		pieceHashes = append(pieceHashes, []byte(t.metaInfo.Info.Pieces[offset:offset+20]))
	}

	diskIO := NewDiskIO(t.metaInfo)
	diskIO.Init()
	pieces := diskIO.Verify()

	go diskIO.Run()

	totalLength := 0
	numFiles := 0
	if t.metaInfo.Info.Length != 0 {
		// There is a single file
		totalLength = t.metaInfo.Info.Length
		numFiles = 1
	} else {
		// There are multiple files
		for i := 0; i < len(t.metaInfo.Info.Files); i++ {
			totalLength += t.metaInfo.Info.Files[i].Length
			numFiles += 1
		}
	}

	log.Printf("Torrent : Run : The torrent contains %d file(s), which are split across %d pieces", numFiles, (len(t.metaInfo.Info.Pieces) / 20))
	log.Printf("Torrent : Run : The total length of all file(s) is %d", totalLength)

	server := NewServer()
	stats := NewStats()
	trackerManager := NewTrackerManager(server.Port)
	peerManager := NewPeerManager(t.infoHash, len(pieceHashes), t.metaInfo.Info.PieceLength, totalLength, diskIO.peerChans, server.peerChans, stats.peerCh, trackerManager.peerChans)
	controller := NewController(pieces, pieceHashes, diskIO.contChans, peerManager.contChans, peerManager.peerContChans)

	go controller.Run()
	go stats.Run()
	go peerManager.Run()
	go server.Run()
	go trackerManager.Run(t.metaInfo, t.infoHash)

	for {
		select {
		case <-t.t.Dying():
			server.Stop()
			peerManager.Stop()
			trackerManager.Stop()
			diskIO.Stop()
			return
		}
	}
}
