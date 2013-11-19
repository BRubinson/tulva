// Copyright 2013 Jari Takkala. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"launchpad.net/tomb"
	"log"
	"net"
	"sort"
//	"syscall"
)

// PeerTuple represents a single IP+port pair of a peer
type PeerTuple struct {
	IP   net.IP
	Port uint16
}

type Peer struct {
	peer PeerTuple
	conn net.Conn
}

type PeerManager struct {
	peersCh <-chan PeerTuple
	statsCh chan Stats
	connsCh <-chan *net.TCPConn
	peers	map[string]*Peer
	t       tomb.Tomb
}

type PeerInfo struct {
	peerId          string
	isChoked        bool // The peer is connected but choked. Defaults to TRUE (choked)
	availablePieces []bool
	activeRequests  map[int]struct{}
	qtyPiecesNeeded int                 // The quantity of pieces that this peer has that we haven't yet downloaded.
	requestPieceCh  chan<- RequestPiece // Other end is Peer. Used to tell the peer to request a particular piece.
	cancelPieceCh   chan<- CancelPiece  // Other end is Peer. Used to tell the peer to cancel a particular piece.
	havePieceCh		chan<- chan<- HavePiece 	// Other end is Peer. Used to give the peer the initial bitfield and new pieces. 
}

type SortedPeers []PeerInfo

func (sp SortedPeers) Less(i, j int) bool {
	return sp[i].qtyPiecesNeeded <= sp[j].qtyPiecesNeeded
}

func (sp SortedPeers) Swap(i, j int) {
	tmp := sp[i]
	sp[i] = sp[j]
	sp[j] = tmp
}

func (sp SortedPeers) Len() int {
	return len(sp)
}

func sortedPeersByQtyPiecesNeeded(peers map[string]PeerInfo) SortedPeers {
	peerInfoSlice := make(SortedPeers, 0)

	for _, peerInfo := range peers {
		peerInfoSlice = append(peerInfoSlice, peerInfo)
	}
	sort.Sort(peerInfoSlice)

	return peerInfoSlice
}

func NewPeerManager(peersCh chan PeerTuple, statsCh chan Stats, connsCh chan *net.TCPConn) *PeerManager {
	pm := new(PeerManager)
	pm.peersCh = peersCh
	pm.statsCh = statsCh
	pm.connsCh = connsCh
	pm.peers = make(map[string]*Peer)
	return pm
}

func NewPeerInfo(quantityOfPieces int) *PeerInfo {
	pi := new(PeerInfo)
	pi.availablePieces = make([]bool, quantityOfPieces)
	pi.activeRequests = make(map[int]struct{})

	// FIXME Not finished. Need to hook these channels into the Peer struct
	pi.requestPieceCh = make(chan<- RequestPiece)
	pi.cancelPieceCh = make(chan<- CancelPiece)
	return pi
}

func NewPeer(peerTuple PeerTuple) *Peer {
	peer := new(Peer)
	raddr := net.TCPAddr { peerTuple.IP, int(peerTuple.Port), "" }
	/*
	go func() {
	conn, err := net.DialTCP("tcp4", nil, &raddr)
	if err != nil {
		if e, ok := err.(*net.OpError); ok {
			if e.Err == syscall.ECONNREFUSED {
				fmt.Printf("%#q\n", e)
				return
			}
		}
		log.Fatal(err)
	}
	fmt.Println(conn)
	}()
	*/
	fmt.Println(raddr)

	return peer
}

/*
func NewPeerConn(net.Conn) {
}

func NewPeerTuple() {
}
*/

func (pm *PeerManager) Stop() error {
	log.Println("PeerManager : Stop : Stopping")
	pm.t.Kill(nil)
	return pm.t.Wait()
}

func (pm *PeerManager) Run() {
	log.Println("PeerManager : Run : Started")
	defer pm.t.Done()
	defer log.Println("PeerManager : Run : Completed")

	for {
		select {
		case peer := <-pm.peersCh:
			peerID := fmt.Sprintf("%s:%d", peer.IP.String(), peer.Port)
			_, ok := pm.peers[peerID]
			if ok {
				// Peer already exists
				log.Printf("Peer %s already in map\n", peerID)
			} else {
				pm.peers[peerID] = NewPeer(peer)
			}
		case conn := <-pm.connsCh:
			fmt.Println(conn)
		case <-pm.t.Dying():
			return
		}
	}
}
