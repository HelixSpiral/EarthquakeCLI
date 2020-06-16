package main

import (
	"encoding/json" // Needed to parse USGS data
	"fmt"           // Needed for printing
	"io/ioutil"     // Needed to read data from the USGS website
	"net/http"      // Needed to query the USGS website
	"strconv"       // Needed to convert strings to a float
	"time"          // Needed to parse the unix timestamp from USGS

	"github.com/gdamore/tcell"
	"github.com/rivo/tview"
)

const (
	USGSAPI    = "https://earthquake.usgs.gov/earthquakes/feed/v1.0/summary/all_hour.geojson"
	TIMEFORMAT = "Jan/02/15:04:05/MST"
)

// Structs for holding the GeoJson information
// From the USGS: https://tools.ietf.org/html/rfc7946
type geoJson struct {
	Type     string           `json:"type"`
	Metadata geoJsonMetadata  `json:"metadata"`
	Features []geoJsonFeature `json:"features"`
}

type geoJsonMetadata struct {
	Generated int64  `json:"generated"`
	Url       string `json:"url"`
	Title     string `json:"title"`
	Api       string `json:"api"`
	Count     int    `json:"count"`
	Status    int    `json:"status"`
}

type geoJsonFeature struct {
	Type       string            `json:"type"`
	Properties geoJsonProperties `json:"properties"`
	Geometry   geoJsonGeometry   `json:"geometry"`
	ID         string            `json:"id"`
}

type geoJsonProperties struct {
	Mag     float64 `json:"mag"`
	Place   string  `json:"place"`
	Time    int64   `json:"time"`
	Updated int64   `json:"updated"`
	Tz      int64   `json:"tz"`
	URL     string  `json:"url"`
	Detail  string  `json:"detail"`
	Felt    int64   `json:"felt"`
	Cdi     float64 `json:"cdi"`
	Mmi     float64 `json:"mmi"`
	Alert   string  `json:"alert"`
	Status  string  `json:"status"`
	Tsunami int     `json:"tsunami"`
	Sig     int     `json:"sig"`
	Net     string  `json:"net"`
	Code    string  `json:"code"`
	Ids     string  `json:"ids"`
	Sources string  `json:"sources"`
	Types   string  `json:"types"`
	Nst     int     `json:"nst"`
	Dmin    float64 `json:"dmin"`
	Rms     float64 `json:"rms"`
	Gap     float64 `json:"gap"`
	MagType string  `json:"magType"`
	Type    string  `json:"type"`
	Title   string  `json:"title"`
}

type geoJsonGeometry struct {
	Type        string    `json:"type"`
	Coordinates []float64 `json:"coordinates"`
}

func main() {

	// Create the new app and table
	app := tview.NewApplication()
	table := tview.NewTable().SetBorders(true).SetSelectable(true, false).SetFixed(1, 0)

	// We store the quakes we've already put in the table so we don't get dupes
	quakesSeen := make(map[int64]string)

	// Populate with initial layout data
	for column := 0; column < 4; column++ {
		color := tcell.ColorYellow
		align := tview.AlignCenter
		text := ""
		switch column {
		case 0:
			text = "Time"
		case 1:
			text = "Magnitude"
		case 2:
			text = "Location"
		case 3:
			text = "Coordinates"
		}
		table.SetCell(0,
			column,
			&tview.TableCell{
				Text:          text,
				Color:         color,
				Align:         align,
				NotSelectable: true,
			})
	}

	// Run updating the table in a go routine
	go func(app *tview.Application, table *tview.Table, quakesSeen map[int64]string) {
		// We have to do an initial populate because the updateTick takes a minute
		populateTableData(app, table, quakesSeen)

		// Tickers to redraw the app and update with new data
		updateTick := time.NewTicker(time.Minute).C
		drawTick := time.NewTicker(time.Second).C
		for {
			select {
			case <-drawTick:
				app.Draw()
			case <-updateTick:
				populateTableData(app, table, quakesSeen)
			}
		}
	}(app, table, quakesSeen)

	if err := app.SetRoot(table, true).Run(); err != nil {
		panic(err)
	}
}

// Add a new row to the table
func addRow(app *tview.Application, table *tview.Table, quake []string) {
	var rowToInsert int
	var quakeTime time.Time
	var rowTime time.Time
	var quakeMag float64

	rowToInsert = 1
	quakeTime, _ = time.Parse(TIMEFORMAT, quake[0])
	quakeMag, _ = strconv.ParseFloat(quake[1], 32)

	// Loop for the table rows to figure out where to put the new row based on the time it happened
	// Occasionally we'll get a 10-15 minute old quake we haven't seen from the USGS, not sure why
	for row := 1; row < table.GetRowCount(); row++ {
		rowTime, _ = time.Parse(TIMEFORMAT, table.GetCell(row, 0).Text)

		if rowTime.Before(quakeTime) {
			rowToInsert = row
			break
		}
	}

	// Actually update the table
	app.QueueUpdateDraw(func() {
		table.InsertRow(rowToInsert)
		for column := 0; column < 4; column++ {
			color := tcell.ColorGreen
			switch {
			case quakeMag >= 4 && quakeMag <= 5.99:
				color = tcell.ColorYellow
			case quakeMag >= 6 && quakeMag <= 6.99:
				color = tcell.ColorOrange
			case quakeMag >= 7:
				color = tcell.ColorRed
			}
			align := tview.AlignLeft
			if column == 0 {
				color = tcell.ColorDarkCyan
			}
			table.SetCell(rowToInsert,
				column,
				&tview.TableCell{
					Text:          quake[column],
					Color:         color,
					Align:         align,
					NotSelectable: column == 0,
				})
		}
	})
}

// Get the list of quakes and update the table
func populateTableData(app *tview.Application, table *tview.Table, quakesSeen map[int64]string) {
	quakeList := getQuakeList(quakesSeen)

	for _, y := range quakeList {
		addRow(app, table, y)
	}
}

// Get the list of quakes
func getQuakeList(quakesSeen map[int64]string) [][]string {
	var quakeList [][]string

	data := getUsgsGeoStats(USGSAPI)

	// Newest results on the bottom so we can loop and insert at the top
	for i := len(data.Features)/2 - 1; i >= 0; i-- {
		opp := len(data.Features) - 1 - i
		data.Features[i], data.Features[opp] = data.Features[opp], data.Features[i]
	}

	// Loop over all the quakes in the list and get the data we want from them.
	for _, y := range data.Features {
		if quakesSeen[y.Properties.Time] == y.Properties.URL {
			continue
		}
		quakesSeen[y.Properties.Time] = y.Properties.URL
		quakeList = append(quakeList,
			[]string{
				time.Unix(y.Properties.Time/1000, 0).Format("Jan/02/15:04:05/MST"),
				fmt.Sprintf("%.02f", y.Properties.Mag),
				y.Properties.Place,
				fmt.Sprintf("%f %f %f", y.Geometry.Coordinates[0], y.Geometry.Coordinates[1], y.Geometry.Coordinates[2]),
			})
	}

	return quakeList
}

// Query the USGS API
func getUsgsGeoStats(url string) geoJson {
	var jsonData geoJson
	resp, err := http.Get(url)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)

	err = json.Unmarshal(body, &jsonData)
	if err != nil {
		panic(err)
	}

	return jsonData
}
