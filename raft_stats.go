package main

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/coreos/go-raft"
)

const (
	queueCapacity = 200
)

type runtimeStats struct {
}

type packageStats struct {
	sendingTime time.Time
	size        int
}

func NewPackageStats(now time.Time, size int) *packageStats {
	return &packageStats{
		sendingTime: now,
		size:        size,
	}
}

func (ps *packageStats) Time() time.Time {
	return ps.sendingTime
}

type raftServerStats struct {
	State                 string
	StartTime             time.Time
	Leader                string
	leaderStartTime       time.Time
	LeaderUptime          string
	RecvAppendRequestCnt  uint64
	SendAppendRequestCnt  uint64
	SendAppendReqeustRate uint64
	sendRateQueue         *statsQueue
	recvRateQueue         *statsQueue
	SendingPkgRate        float64
	SendingBandwidthRate  float64
	RecvingPkgRate        float64
	RecvingBandwidthRate  float64
}

func (ss *raftServerStats) RecvAppendReq(leaderName string, pkgSize int) {
	ss.State = raft.Follower
	if leaderName != ss.Leader {
		ss.Leader = leaderName
		ss.leaderStartTime = time.Now()
	}

	ss.recvRateQueue.Insert(NewPackageStats(time.Now(), pkgSize))
	ss.RecvAppendRequestCnt++
}

func (ss *raftServerStats) SendAppendReq(pkgSize int) {
	now := time.Now()
	if ss.State != raft.Leader {
		ss.State = raft.Leader
		ss.Leader = r.Name()
		ss.leaderStartTime = now
	}

	ss.sendRateQueue.Insert(NewPackageStats(time.Now(), pkgSize))

	ss.SendAppendRequestCnt++
}

type raftPeerStats struct {
	Latency          float64 `json:"latency"`
	AvgLatency       float64 `json:"averageLatency"`
	avgLatencySquare float64
	SdvLatency       float64 `json:"sdvLatency"`
	MinLatency       float64 `json:"minLatency"`
	MaxLatency       float64 `json:"maxLatency"`
	FailCnt          uint64  `json:"failsCount"`
	SuccCnt          uint64  `json:"successCount"`
}

func (ps *raftPeerStats) Fail() {
	ps.FailCnt++
}

func (ps *raftPeerStats) Succ(d time.Duration) {

	total := float64(ps.SuccCnt) * ps.AvgLatency
	totalSquare := float64(ps.SuccCnt) * ps.avgLatencySquare

	ps.SuccCnt++

	ps.Latency = float64(d) / (1000000.0)

	if ps.Latency > ps.MaxLatency {
		ps.MaxLatency = ps.Latency
	}

	if ps.Latency < ps.MinLatency {
		ps.MinLatency = ps.Latency
	}

	ps.AvgLatency = (total + ps.Latency) / float64(ps.SuccCnt)
	ps.avgLatencySquare = (totalSquare + ps.Latency*ps.Latency) / float64(ps.SuccCnt)

	// sdv = sqrt(avg(x^2) - avg(x)^2)
	ps.SdvLatency = math.Sqrt(ps.avgLatencySquare - ps.AvgLatency*ps.AvgLatency)
}

type statsQueue struct {
	items        [queueCapacity]*packageStats
	size         int
	front        int
	back         int
	totalPkgSize int
	rwl          sync.RWMutex
}

func (q *statsQueue) Len() int {
	return q.size
}

func (q *statsQueue) Size() int {
	return q.totalPkgSize
}

// FrontAndBack gets the front and back elements in the queue
// We must grab front and back together with the protection of the lock
func (q *statsQueue) frontAndBack() (*packageStats, *packageStats) {
	q.rwl.RLock()
	defer q.rwl.RUnlock()
	if q.size != 0 {
		return q.items[q.front], q.items[q.back]
	}
	return nil, nil
}

func (q *statsQueue) Insert(p *packageStats) {
	q.rwl.Lock()
	defer q.rwl.Unlock()

	if q.size == queueCapacity { //dequeue
		q.totalPkgSize -= q.items[q.front].size
		q.front = (q.back + 2) % queueCapacity
	} else {
		q.size++
	}

	q.back = (q.back + 1) % queueCapacity
	q.items[q.back] = p
	q.totalPkgSize += q.items[q.back].size

	fmt.Println(q.front, q.back, q.size)
}

func (q *statsQueue) Rate() (float64, float64) {
	front, back := q.frontAndBack()

	if front == nil || back == nil {
		return 0, 0
	}

	sampleDuration := back.Time().Sub(front.Time())

	pr := float64(q.Len()) / float64(sampleDuration) * float64(time.Second)

	br := float64(q.Size()) / float64(sampleDuration) * float64(time.Second)

	return pr, br
}
