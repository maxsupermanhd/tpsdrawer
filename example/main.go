package main

import (
	"database/sql"
	_ "embed"
	"encoding/gob"
	"flag"
	"fmt"
	"image/color"
	"image/png"
	"log"
	"os"
	"sort"
	"time"
	"tpsdrawer"

	_ "github.com/mattn/go-sqlite3"
	"github.com/mazznoer/colorgrad"
)

var (
	from       = flag.String("from", "sqlite", "sqlite or gob")
	dbpath     = flag.String("f", "db.sqlite3", "path to db")
	mode       = flag.String("mode", "percentile", "coloring mode: percentile or average")
	sqlite2gob = flag.Bool("sqlite2gob", false, "writes gob from sql data")
)

func main() {
	flag.Parse()

	modeFN := percentileSlice
	switch *mode {
	case "percentile":
	case "average":
		modeFN = avgSlice
	default:
		panic("wrong mode")
	}

	timedata := []time.Time{}
	tpsdata := []float64{}

	switch *from {
	case "sqlite":
		_, err := os.Stat(*dbpath)
		if os.IsNotExist(err) {
			panic("database doesn't exist")
		}
		log.Println("Opening database...")
		db := noerr(sql.Open("sqlite3", *dbpath))
		log.Println("Getting data...")
		timedata, tpsdata = noerr2(GetTPSValues(db))
		if *sqlite2gob {
			log.Println("Writing time data...")
			f := noerr(os.Create("timedata.gob"))
			must(gob.NewEncoder(f).Encode(timedata))
			must(f.Close())
			log.Println("Writing tps data...")
			f = noerr(os.Create("tpsdata.gob"))
			must(gob.NewEncoder(f).Encode(tpsdata))
			must(f.Close())
		}
	case "gob":
		log.Println("Opening time data...")
		f := noerr(os.Open("timedata.gob"))
		log.Println("Decoding time data...")
		must(gob.NewDecoder(f).Decode(&timedata))
		f.Close()
		log.Println("Opening tps data...")
		f = noerr(os.Open("tpsdata.gob"))
		log.Println("Decoding tps data...")
		must(gob.NewDecoder(f).Decode(&tpsdata))
		f.Close()
	default:
		panic("wrong from")
	}

	log.Println("Drawing...")
	grad := noerr(colorgrad.NewGradient().
		HtmlColors("darkred", "gold", "green").
		Domain(0, 20).
		Build())

	i := tpsdrawer.DrawTPS(timedata, tpsdata, tpsdrawer.DrawOptions{
		DayW:       250,
		DayH:       50,
		Padding:    10,
		Background: color.RGBA{R: 0x18, G: 0x18, B: 0x18, A: 0xFF},
		Spacing:    4,
		FontColor:  color.White,
		Debug:      true,
		Gradient: func(f float64) color.Color {
			if f == 0 {
				return color.RGBA{R: 0x33, G: 0x33, B: 0x33, A: 0xFF}
			}
			r, g, b := grad.At(f).RGB255()
			return color.RGBA{R: r, G: g, B: b, A: 0xFF}
		},
		SampleH:     20,
		Comment:     fmt.Sprint("Made by FlexCoral, tracked by Yokai0nTop, ", len(timedata), " samples"),
		BreakMonths: true,
		BreakMonday: false,
		MeasureFunc: modeFN,
	})
	log.Println("Writing image...")
	o := noerr(os.Create("tps.png"))
	defer o.Close()
	must(png.Encode(o, i))
	log.Println("Done!")
}

func avgSlice(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := 0.0
	for i := 0; i < len(v); i++ {
		s += v[i]
	}
	return s / float64(len(v))
}

func percentileSlice(c []float64) (percentile float64) {
	percent := 1.0
	if len(c) == 0 {
		return 0
	}
	if len(c) == 1 {
		return c[0]
	}
	sort.Float64s(c)
	index := (percent / 100) * float64(len(c))
	if index == float64(int64(index)) {
		i := int(index)
		return c[i-1]
	} else if index > 1 {
		i := int(index)
		return c[i-1] + c[i]/float64(len(c))
	} else {
		return 0
	}
}

func GetAvgTPSValues(db *sql.DB) ([]time.Time, []float64, error) {
	rows, err := db.Query(`select cast(whenlogged as int), tpsvalue from tps order by whenlogged asc;`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	tpsval := []time.Time{}
	tpsn := []float64{}
	for rows.Next() {
		var (
			when int64
			tps  float64
		)
		err = rows.Scan(&when, &tps)
		if err != nil {
			return nil, nil, err
		}
		tpsunix := time.Unix(when, 0)
		tpsavgs := float64(0)
		tpsavgc := float64(0)
		timeavg := 20
		ticksavg := timeavg * 20
		for i := len(tpsn); i > 0 && i+ticksavg < len(tpsn); i++ {
			if tpsunix.Sub(tpsval[i]) > time.Duration(timeavg)*time.Second {
				break
			}
			tpsavgc++
			tpsavgs += tpsn[i]
		}
		tpsval = append(tpsval, tpsunix)
		if tpsavgc > 0 {
			tpsn = append(tpsn, tpsavgs/tpsavgc)
		} else {
			tpsn = append(tpsn, tps)
		}
	}
	return tpsval, tpsn, nil
}

func GetTPSValues(db *sql.DB) ([]time.Time, []float64, error) {
	var c int64
	log.Println("Counting samples...")
	err := db.QueryRow(`select count(*) from tps order by whenlogged asc;`).Scan(&c)
	if err != nil {
		return nil, nil, err
	}
	log.Println("Allocating space for", c, "samples...")
	tpsval := make([]time.Time, 0, c)
	tpsn := make([]float64, 0, c)
	log.Println("Getting samples...")
	rows, err := db.Query(`select cast(whenlogged as int), tpsvalue from tps order by whenlogged asc;`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	scount := 0
	for rows.Next() {
		if scount%(24*60*60*7) == 0 && scount != 0 {
			log.Println("Got", scount, "samples...")
		}
		scount++
		var (
			when int64
			tps  float64
		)
		err = rows.Scan(&when, &tps)
		if err != nil {
			return nil, nil, err
		}
		tpsunix := time.Unix(when, 0)
		tpsval = append(tpsval, tpsunix)
		tpsn = append(tpsn, tps)
	}
	return tpsval, tpsn, nil
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func noerr[T any](ret T, err error) T {
	must(err)
	return ret
}

func noerr2[T1 any, T2 any](ret1 T1, ret2 T2, err error) (T1, T2) {
	must(err)
	return ret1, ret2
}
