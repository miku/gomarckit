package main

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime/pprof"
	"strconv"
	"strings"

	"github.com/ubleipzig/marctools"
)

func main() {
	secondary := flag.String("secondary", "", "add a secondary value to the row")
	encodeRecord := flag.Bool("encode", false, "base64 encode record before inserting it")
	output := flag.String("o", "", "output sqlite3 filename")
	version := flag.Bool("v", false, "prints current program version")
	cpuprofile := flag.String("cpuprofile", "", "write cpu profile to file")
	safe := flag.Bool("safe", false, "use slower, but safer method to extract record identifiers")

	var PrintUsage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS] MARCFILE\n", os.Args[0])
		flag.PrintDefaults()
	}

	flag.Parse()

	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	if *version {
		fmt.Println(marctools.AppVersion)
		os.Exit(0)
	}

	if flag.NArg() < 1 {
		PrintUsage()
		os.Exit(1)
	}

	filename := flag.Args()[0]

	// record ids in order
	var b bytes.Buffer
	marctools.MarcMap(filename, &b, *safe)

	// the input file
	handle, err := os.Open(filename)
	if err != nil {
		log.Fatalf("%s\n", err)
	}

	defer func() {
		if err := handle.Close(); err != nil {
			log.Fatalf("%s\n", err)
		}
	}()

	// prepare sqlite3 output
	db, err := sql.Open("sqlite3", *output)
	if err != nil {
		log.Fatalln(err)
	}
	defer db.Close()

	// prepare table
	init := `CREATE TABLE IF NOT EXISTS store (id TEXT, secondary TEXT, record BLOB, PRIMARY KEY (id, secondary))`
	_, err = db.Exec(init)
	if err != nil {
		log.Fatalf("%q: %s\n", err, init)
	}

	// prepare statement
	tx, err := db.Begin()
	if err != nil {
		log.Fatalln(err)
	}
	stmt, err := tx.Prepare("INSERT INTO store VALUES (?, ?, ?)")
	if err != nil {
		log.Fatalln(err)
	}
	defer stmt.Close()

	for _, row := range strings.Split(b.String(), "\n") {
		fields := strings.Fields(row)
		if len(fields) == 0 {
			continue
		}

		offset, err := strconv.Atoi(fields[1])
		if err != nil {
			log.Fatalln(err)
		}

		length, err := strconv.Atoi(fields[2])
		if err != nil {
			log.Fatalln(err)
		}

		handle.Seek(int64(offset), os.SEEK_SET)
		buf := make([]byte, length)
		_, err = handle.Read(buf)
		if err != nil {
			log.Fatalln(err)
		}

		var s string
		if *encodeRecord {
			s = base64.StdEncoding.EncodeToString(buf)
		} else {
			s = string(buf)
		}
		_, err = stmt.Exec(fields[0], *secondary, s)
		if err != nil {
			log.Fatalln(err)
		}
	}

	// create index
	_, err = tx.Exec("CREATE INDEX IF NOT EXISTS idx_store_id ON store (id)")
	if err != nil {
		log.Fatalln(err)
	}
	tx.Commit()
}
