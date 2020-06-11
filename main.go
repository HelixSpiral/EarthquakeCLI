package main

import (
	"encoding/json" // Needed to parse USGS data
	"fmt"           // Needed for printing
	"io/ioutil"     // Needed to read data from the USGS website
	"net/http"      // Needed to query the USGS website
	"regexp"        // Needed to parse the location data
	"strings"       // Needed for strings.ToUpper
	"time"          // Needed to parse the unix timestamp from USGS

	"github.com/gdamore/tcell"
	"github.com/rivo/tview"
)

const (
	USGSAPI = "https://earthquake.usgs.gov/earthquakes/feed/v1.0/summary/all_hour.geojson"
	TIMEFORMAT = "Jan/02/15:04:05/MST"
	)

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

	app := tview.NewApplication()
	table := tview.NewTable().SetBorders(true).SetSelectable(true, false).SetFixed(1,0)

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

	quakesSeen := make(map[int64]string)

	go func(app *tview.Application, table *tview.Table, quakesSeen map[int64]string) {
		populateTableData(app, table, quakesSeen)
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

func addRow(app *tview.Application, table *tview.Table, quake []string) {
	var rowToInsert int
	var quakeTime time.Time
	var rowTime time.Time

	rowToInsert = 1
	quakeTime, _ = time.Parse(TIMEFORMAT, quake[0])

	for row := 1; row < table.GetRowCount(); row++ {
		rowTime, _ = time.Parse(TIMEFORMAT, table.GetCell(row, 0).Text)

		if rowTime.Before(quakeTime) {
			rowToInsert = row
			break
		}
	}
	app.QueueUpdateDraw(func() {
		table.InsertRow(rowToInsert)
		for column := 0; column < 4; column++ {
			color := tcell.ColorGreen
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

func populateTableData(app *tview.Application, table *tview.Table, quakesSeen map[int64]string) {
	quakeList := getQuakeList(quakesSeen)

	for _, y := range quakeList {
		addRow(app, table, y)
	}
}

func getQuakeList(quakesSeen map[int64]string) [][]string {
	var quakeList [][]string

	data := getUsgsGeoStats(USGSAPI)

	// Newest results on the bottom so we can loop and insert at the top
	for i := len(data.Features)/2 - 1; i >= 0; i-- {
		opp := len(data.Features) - 1 - i
		data.Features[i], data.Features[opp] = data.Features[opp], data.Features[i]
	}

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

// Unused legacy func
func displayFeature(geoFeat *geoJsonFeature) {
	locationRegex := regexp.MustCompile(`(\d{1,3}\s?km)\s([NESW]+)\sof\s([^.+]+)$`)
	locationSplit := locationRegex.FindStringSubmatch(geoFeat.Properties.Place)

	// Sometimes we get fucked location data, if we do just print the unparsed location.
	if len(locationSplit) < 3 {
		fmt.Printf("[%s|%v] %s\r\n", strings.ToUpper(geoFeat.Properties.Type), time.Unix(geoFeat.Properties.Time/1000, 0).Format("2006/Jan/02/15:04:05/MST"), geoFeat.Properties.Place)
	} else {
		fmt.Printf("[%s|%v] %s (%s %s)\r\n", strings.ToUpper(geoFeat.Properties.Type), time.Unix(geoFeat.Properties.Time/1000, 0).Format("2006/Jan/02/15:04:05/MST"), locationSplit[3], locationSplit[1], locationSplit[2])
	}
	fmt.Println("\t- Magnitude:", geoFeat.Properties.Mag)
	fmt.Printf("\t- Coords: [Long: %f, Lat: %f, Z: %f\r\n", geoFeat.Geometry.Coordinates[0], geoFeat.Geometry.Coordinates[1], geoFeat.Geometry.Coordinates[2])
}
