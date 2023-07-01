package tpsdrawer

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"time"

	"github.com/fogleman/gg"
	"golang.org/x/image/font"
)

type DrawOptions struct {
	DayW        uint
	DayH        uint
	Padding     uint
	Spacing     uint
	Background  color.Color
	FontColor   color.Color
	Font        font.Face
	Debug       bool
	Gradient    func(float64) color.Color
	SampleH     uint
	Comment     string
	BreakMonths bool
	BreakMonday bool
	MeasureFunc func([]float64) float64
}

func DrawTPS(tpsK []time.Time, tpsV []float64, opts DrawOptions) image.Image {
	if len(tpsK) != len(tpsV) {
		panic("tpsK and tpsV not same length")
	}
	if len(tpsK) == 0 {
		panic("Nothing to draw")
	}

	monthW, monthH, fontNumbersH := measureFont(opts.Font)

	// log.Println("Analyzing data...")
	days := analyzeTPS(tpsK, tpsV, int(opts.DayW), opts.MeasureFunc)

	monthRows := 0
	currMonth := ""
	if opts.BreakMonths {
		for _, d := range days {
			if currMonth != d.t.Month().String() {
				currMonth = d.t.Month().String()
				if opts.BreakMonths {
					if opts.BreakMonday {
						if d.t.Weekday() != 0 {
							monthRows++
						}
					} else {
						monthRows++
					}
				}
			}
		}
	}
	if tpsK[0].Day() != 1 {
		monthRows++
	}

	imgW := int(opts.Padding*2 + uint(monthW) + opts.Spacing*2 + 1 + 7*opts.DayW + 6*opts.Spacing)
	imgH := int(opts.Padding*4 + (opts.DayH+opts.Spacing)*uint(len(days)/7+monthRows) + opts.SampleH + uint(monthH*2))

	c := gg.NewContext(imgW, imgH)
	c.SetColor(opts.Background)
	c.Clear()
	if opts.Font != nil {
		c.SetFontFace(opts.Font)
	}

	gridX := monthW + float64(opts.Padding)
	gridY := float64(opts.Padding)

	c.SetColor(opts.FontColor)
	c.DrawString(opts.Comment, float64(opts.Padding), float64(imgH-int(opts.Padding)))

	sampleW := imgW - int(opts.Padding*2)
	for i := 0; i < sampleW; i++ {
		cr, cg, cb, _ := opts.Gradient((float64(i) / float64(sampleW)) * 20).RGBA()
		c.SetRGB255(int(cr), int(cg), int(cb))
		c.DrawLine(float64(int(opts.Padding)+i), float64(uint(imgH)-opts.Padding*2-uint(monthH*2)),
			float64(int(opts.Padding)+i), float64(uint(imgH)-opts.Padding*2-uint(monthH*2)-opts.SampleH))
		c.Stroke()
	}

	currWeek := getWeekNum(tpsK[0])
	rowN := 0
	currMonth = tpsK[0].Month().String()
	c.SetColor(opts.FontColor)
	c.DrawString(tpsK[0].Month().String(), float64(opts.Padding), float64(rowN*int(opts.DayH+opts.Spacing))+monthH*2)
	for _, d := range days {
		k := d.t
		kwn := getWeekNum(k)
		kd := k.Day()
		kw := k.Weekday()
		if kw == 0 {
			kw = 6
		} else {
			kw--
		}
		if kwn != currWeek {
			currWeek = kwn
			// log.Println("Week", currWeek)
			rowN++
		}
		if currMonth != k.Month().String() {
			currMonth = k.Month().String()
			if opts.BreakMonths {
				if opts.BreakMonday {
					if kw != 0 {
						rowN++
					}
				} else {
					rowN++
				}
			}
			c.SetColor(opts.FontColor)
			c.DrawString(k.Month().String(), float64(opts.Padding), float64(rowN*int(opts.DayH+opts.Spacing))+monthH*2)
			// log.Println("Month", d.t.String())
		}

		rx := gridX + float64(int(kw)*int(opts.DayW+opts.Spacing))
		ry := gridY + float64(rowN*int(opts.DayH+opts.Spacing))
		rw := float64(opts.DayW)
		rh := float64(opts.DayH)

		// log.Println(rx, ry, rw, rh)

		c.SetColor(opts.FontColor)
		c.DrawString(fmt.Sprint(kd), rx+4, ry+fontNumbersH+1)
		if opts.Debug {
			c.DrawString(fmt.Sprint(kwn), rx+4, ry+fontNumbersH*2+4)
			c.DrawString(fmt.Sprint(int(k.Month())), rx+4, ry+fontNumbersH*3+8)
			c.SetColor(color.White)
			c.DrawRectangle(rx, ry, rw, rh)
			c.Stroke()
		}

		for i, v := range d.vals {
			cr, cg, cb, _ := opts.Gradient(v).RGBA()
			c.SetRGB255(int(cr), int(cg), int(cb))
			c.DrawLine(rx+float64(i), ry, rx+float64(i), ry+rh)
			c.Stroke()
		}
	}

	return c.Image()
}

type measuredDay struct {
	t    time.Time
	vals []float64
}

func analyzeTPS(tpsK []time.Time, tpsV []float64, dayWidth int, measureFunc func([]float64) float64) []measuredDay {
	b := truncateTime(tpsK[0])
	e := tpsK[len(tpsK)-1]
	m := []measuredDay{}
	skip := 0
	for {
		if b.After(e) {
			return m
		}
		nb := b.AddDate(0, 0, 1)

		dk := []time.Time{}
		dv := []float64{}
		for i := skip; i < len(tpsK); i++ {
			v := tpsK[i]
			if v.After(nb) {
				break
			}
			if v.After(b) && v.Before(nb) {
				dk = append(dk, v)
				dv = append(dv, tpsV[i])
				skip = i
			}
		}

		m = append(m, measuredDay{
			t:    b,
			vals: measureDay(dk, dv, b, dayWidth, measureFunc),
		})

		b = nb
		// log.Println("Analyzed", len(m), "days, sample", skip)
	}
}

func measureDay(tpsK []time.Time, tpsV []float64, begin time.Time, dayWidth int, measureFunc func([]float64) float64) []float64 {
	ret := make([]float64, dayWidth)
	if len(tpsK) == 0 {
		return ret
	}
	sample := time.Duration(int64(time.Hour*24) / int64(dayWidth))
	current := begin
	skip := 0
	for s := 0; s < dayWidth; s++ {
		a := []float64{}
		nc := current.Add(sample)
		for i := skip; i < len(tpsK); i++ {
			if tpsK[i].After(nc) {
				break
			}
			if tpsK[i].After(current) && tpsK[i].Before(nc) {
				skip = i
				a = append(a, tpsV[i])
			}
		}
		current = nc
		ret[s] = measureFunc(a)
	}
	return ret
}

func truncateTime(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func getWeekNum(t time.Time) int {
	y, w := t.ISOWeek()
	return w + int(math.Round(float64(y)*52.1429))
}

func measureFont(f font.Face) (monthW, monthH, fontNumbersH float64) {
	mc := gg.NewContext(200, 200)
	if f != nil {
		mc.SetFontFace(f)
	}
	for i := 1; i <= 12; i++ {
		w, h := mc.MeasureString(time.Month(i).String())
		if monthW < w {
			monthW = w
		}
		if monthH < h {
			monthH = h
		}
	}
	_, fontNumbersH = mc.MeasureString("1234567890")
	return
}
