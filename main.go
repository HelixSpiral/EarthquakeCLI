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
	quakeList := make(map[string]geoJsonFeature)

	// Populate with initial layout data
	for column := 0; column < 5; column++ {
		color := tcell.ColorYellow
		align := tview.AlignCenter
		text := ""
		switch column {
		case 0:
			text = "ID"
		case 1:
			text = "Time"
		case 2:
			text = "Magnitude"
		case 3:
			text = "Location"

		// This is just for debugging
		case 4:
			text = "Properties/IDs"
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
	go func(app *tview.Application, table *tview.Table, quakeList map[string]geoJsonFeature) {
		// We have to do an initial populate because the updateTick takes a minute
		populateTableData(app, table, quakeList)

		// Tickers to redraw the app and update with new data
		updateTick := time.NewTicker(time.Minute).C
		drawTick := time.NewTicker(time.Second).C
		for {
			select {
			case <-drawTick:
				app.Draw()
			case <-updateTick:
				populateTableData(app, table, quakeList)
			}
		}
	}(app, table, quakeList)

	if err := app.SetRoot(table, true).Run(); err != nil {
		panic(err)
	}
}

// Add a new row to the table
func addRow(app *tview.Application, table *tview.Table, quake []string) {
	var atRow int
	var rowID string
	var foundTime bool
	var updateNotInsert bool
	var quakeTime time.Time
	var rowTime time.Time
	var quakeMag float64

	atRow = 1
	quakeTime, _ = time.Parse(TIMEFORMAT, quake[1])
	quakeMag, _ = strconv.ParseFloat(quake[2], 32)

	// Loop for the table rows to:
	// A) Check to see if we already have that quake ID in the table somewhere
	// B) Figure out where the quake should go based on the time, if we don't already have it.
	for row := 1; row < table.GetRowCount(); row++ {
		rowTime, _ = time.Parse(TIMEFORMAT, table.GetCell(row, 1).Text)
		rowID = table.GetCell(row, 0).Text

		// If we have that ID in the table, get the row and break.
		if rowID == quake[0] {
			atRow = row
			updateNotInsert = true
			break
		}

		if !foundTime && rowTime.Before(quakeTime) {
			atRow = row
			foundTime = true
		}
	}

	// Actually update the table
	app.QueueUpdateDraw(func() {
		if !updateNotInsert {
			table.InsertRow(atRow)
		}
		for column := 0; column < 5; column++ {
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
			table.SetCell(atRow,
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
func populateTableData(app *tview.Application, table *tview.Table, quakeList map[string]geoJsonFeature) {
	usgsQuakeList := getQuakeList(quakeList)

	for _, y := range usgsQuakeList {
		addRow(app, table, y)
	}
}

// Get the list of quakes
func getQuakeList(quakeList map[string]geoJsonFeature) [][]string {
	var usgsQuakeList [][]string

	data := getUsgsGeoStats(USGSAPI)

	// Newest results on the bottom so we can loop and insert at the top
	for i := len(data.Features)/2 - 1; i >= 0; i-- {
		opp := len(data.Features) - 1 - i
		data.Features[i], data.Features[opp] = data.Features[opp], data.Features[i]
	}

	// Loop over all the quakes in the list and get the data we want from them.
	for _, y := range data.Features {
		/*if quakeList[y.Properties.Time] == y.Properties.URL {
			continue
		}
		*/
		quakeList[y.ID] = y
		usgsQuakeList = append(usgsQuakeList,
			[]string{
				y.ID,
				time.Unix(y.Properties.Time/1000, 0).Format("Jan/02/15:04:05/MST"),
				fmt.Sprintf("%.02f", y.Properties.Mag),
				y.Properties.Place,
				//	fmt.Sprintf("%f %f %f", y.Geometry.Coordinates[0], y.Geometry.Coordinates[1], y.Geometry.Coordinates[2]),
				y.Properties.Ids,
			})
	}

	return usgsQuakeList
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
