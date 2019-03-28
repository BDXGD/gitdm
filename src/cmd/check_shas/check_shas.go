package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

func checkSHAs(files []string) error {
	fmt.Printf("Checking %d files\n", len(files))
	cfn := "./cncf-config/forbidden.csv"
	f, err := os.Open(cfn)
	if err != nil {
		return err
	}
	shas := make(map[string]struct{})
	reader := csv.NewReader(f)
	for {
		row, err := reader.Read()
		if err == io.EOF {
			f.Close()
			break
		} else if err != nil {
			f.Close()
			fmt.Printf("Reading %s\n", cfn)
			return err
		}
		if len(row) != 1 {
			return fmt.Errorf("unexpected row: %+v, it should contain only one column: sha", row)
		}
		if row[0] == "sha" {
			continue
		}
		if len(row[0]) != 64 {
			return fmt.Errorf("unexpected column: %+v, it should have length 64, has: %d", row[0], len(row[0]))
		}
		shas[row[0]] = struct{}{}
	}
	fmt.Printf("Read %d forbiden SHAs\n", len(shas))
	thrN := runtime.NumCPU()
	nThreads := 0
	runtime.GOMAXPROCS(thrN)
	ch := make(chan string)
	lMap := make(map[string][][2]int)
	var lMapMtx sync.Mutex
	nFiles := len(files)
	for idx, file := range files {
		if idx%100 == 99 {
			fmt.Printf("Lines analysis %d/%d\n", idx+1, nFiles)
		}
		go func(c chan string, i int, fn string) {
			data, err := ioutil.ReadFile(fn)
			if err != nil {
				c <- fn + ": " + err.Error()
				return
			}
			lines1 := strings.Split(string(data), "\r")
			lines2 := strings.Split(string(data), "\n")
			lines := lines1
			if len(lines2) > len(lines1) {
				lines = lines2
			}
			for j, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				lMapMtx.Lock()
				v, ok := lMap[line]
				if !ok {
					lMap[line] = [][2]int{{i, j}}
				} else {
					v = append(v, [2]int{i, j})
					lMap[line] = v
				}
				lMapMtx.Unlock()
			}
			c <- ""
		}(ch, idx, file)
		nThreads++
		if nThreads == thrN {
			res := <-ch
			nThreads--
			if res != "" {
				fmt.Printf("%s\n", res)
			}
		}
	}
	for nThreads > 0 {
		res := <-ch
		nThreads--
		if res != "" {
			fmt.Printf("%s\n", res)
		}
	}
	nThreads = 0
	nonWordRE := regexp.MustCompile(`[^\w]`)
	chs := make(chan struct{})
	tMap := make(map[string][][3]int)
	var tMapMtx sync.Mutex
	for line, data := range lMap {
		go func(c chan struct{}, l string, data [][2]int) {
			tokens := nonWordRE.Split(l, -1)
			toks := []string{}
			for _, token := range tokens {
				token = strings.TrimSpace(token)
				if len(token) < 4 {
					continue
				}
				toks = append(toks, token)
			}
			for i, token := range toks {
				tMapMtx.Lock()
				v, ok := tMap[token]
				if !ok {
					v = [][3]int{}
					for _, d := range data {
						v = append(v, [3]int{d[0], d[1], i})
					}
					tMap[token] = v
				} else {
					for _, d := range data {
						v = append(v, [3]int{d[0], d[1], i})
					}
					tMap[line] = v
				}
				tMapMtx.Unlock()
			}
			c <- struct{}{}
		}(chs, line, data)
		nThreads++
		if nThreads == thrN {
			<-chs
			nThreads--
		}
	}
	for nThreads > 0 {
		<-chs
		nThreads--
	}
	keys := []string{}
	for k := range lMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		data := lMap[k]
		fmt.Printf("%s: ", k)
		for _, row := range data {
			fmt.Printf("%s:%d ", files[row[0]], row[1]+1)
		}
		fmt.Printf("\n")
	}
	return nil
}

func main() {
	dtStart := time.Now()
	err := checkSHAs(os.Args[1:])
	dtEnd := time.Now()
	if err != nil {
		fmt.Printf("%+v\n", err)
	}
	fmt.Printf("Time: %v\n", dtEnd.Sub(dtStart))
}
