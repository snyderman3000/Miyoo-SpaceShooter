package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Config lives in data/config.json.
type Config struct {
	Music    bool `json:"music"`
	InvertY  bool `json:"invert_y"`
	PageFlip bool `json:"page_flip"`
	path     string
}

func LoadConfig(dir string) *Config {
	c := &Config{Music: true, PageFlip: true, path: filepath.Join(dir, "config.json")}
	if b, err := os.ReadFile(c.path); err == nil {
		json.Unmarshal(b, c)
	} else {
		c.Save()
	}
	return c
}

func (c *Config) Save() {
	if c.path == "" {
		return
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	writeFileSync(c.path, b)
}

type ScoreEntry struct {
	Score    int    `json:"score"`
	Hits     int    `json:"hits"`
	Survived bool   `json:"survived"`
	Date     string `json:"date"`
}

// ScoreBook keeps the top 5 scores in data/scores.json.
type ScoreBook struct {
	Entries []ScoreEntry `json:"scores"`
	path    string
	now     func() time.Time
}

func LoadScores(dir string) *ScoreBook {
	s := &ScoreBook{path: filepath.Join(dir, "scores.json"), now: time.Now}
	if dir == "" {
		s.path = ""
	}
	if s.path != "" {
		if b, err := os.ReadFile(s.path); err == nil {
			json.Unmarshal(b, s)
		}
	}
	return s
}

func (s *ScoreBook) Best() int {
	if len(s.Entries) == 0 {
		return 0
	}
	return s.Entries[0].Score
}

// Add records a result and returns its place (0 = new high score) or -1.
func (s *ScoreBook) Add(r Result) int {
	if r.Score <= 0 {
		return -1
	}
	e := ScoreEntry{Score: r.Score, Hits: r.Hits, Survived: r.Survived, Date: s.now().Format("Jan 02")}
	rank := 0
	for _, o := range s.Entries {
		if o.Score >= e.Score {
			rank++
		}
	}
	s.Entries = append(s.Entries, e)
	sort.SliceStable(s.Entries, func(i, j int) bool { return s.Entries[i].Score > s.Entries[j].Score })
	if len(s.Entries) > 5 {
		s.Entries = s.Entries[:5]
	}
	if rank >= 5 {
		rank = -1
	}
	if s.path != "" {
		b, _ := json.MarshalIndent(s, "", "  ")
		writeFileSync(s.path, b)
	}
	return rank
}

func writeFileSync(path string, b []byte) {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return
	}
	f.Write(b)
	f.Sync()
	f.Close()
	os.Rename(tmp, path)
}
