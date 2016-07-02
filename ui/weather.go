package ui

import (
	"fmt"
	"log"
	"strings"
	"time"

	"golang.org/x/net/context"

	owm "github.com/briandowns/openweathermap"
	"github.com/studentkittens/eulenfunk/display"
	"github.com/studentkittens/eulenfunk/util"
)

func celsius(c float64) string {
	if c > 100 {
		c = 99.0
	}

	return fmt.Sprintf("%d৹C", int(c))
}

func degToDirection(deg int) string {
	switch {
	case deg > 315 && deg <= 45:
		return "↑"
	case deg <= 135:
		return "→"
	case deg <= 225:
		return "↓"
	case deg <= 315:
		return "←"
	default:
		return "o"
	}
}

func weatherForecast() (*owm.ForecastWeatherData, error) {
	w, err := owm.NewForecast("C", "DE")
	if err != nil {
		log.Fatalln(err)
	}

	err = w.DailyByCoordinates(
		&owm.Coordinates{
			Latitude:  48.3830555,
			Longitude: 10.8830555,
		},
		3, // 3 days of forecast
	)

	if err != nil {
		return nil, err
	}

	return w, err
}

func toScreen(w *owm.ForecastWeatherData, p *owm.ForecastWeatherList, dayOff, width int) []string {
	top := w.City.Name

	now := time.Now()
	date := fmt.Sprintf(
		"%d.%d.%d",
		now.Day()+dayOff,
		now.Month(),
		now.Year()-2000,
	)

	top += strings.Repeat(" ", width-len(top)-len(date))
	top += date

	status := "No weather today."
	if len(p.Weather) > 0 {
		status = util.Center(p.Weather[0].Description, width, ' ')
	}

	humidity := p.Humidity
	if humidity >= 100 {
		humidity = 99
	}

	stats := fmt.Sprintf(
		"R%5.1f%% %2d%% %s%4.1fm/s",
		p.Rain,
		p.Humidity,
		degToDirection(p.Deg),
		p.Speed,
	)

	temps := fmt.Sprintf(
		"%s %s %s %s",
		celsius(p.Temp.Morn),
		celsius(p.Temp.Day),
		celsius(p.Temp.Eve),
		celsius(p.Temp.Night),
	)

	return []string{
		top,
		status,
		stats,
		temps,
	}
}

// RunClock displays the current time in the "clock" window.
func RunWeather(lw *display.LineWriter, width int, ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		w, err := weatherForecast()
		if err != nil {
			log.Printf("Failed to get forecast: %v", err)
			lw.Line("weather", 0, strings.Repeat("=", width))
			lw.Line("weather", 1, "Sorry, no weather today.")
			lw.Line("weather", 2, "Please see the log.")
			lw.Line("weather", 3, strings.Repeat("=", width))
		} else {
			for dayOff, p := range w.List {
				for idx, line := range toScreen(w, &p, dayOff, width) {
					lw.Line("weather", idx, line)
				}
			}
		}

		time.Sleep(15 * time.Minute)
	}
}
