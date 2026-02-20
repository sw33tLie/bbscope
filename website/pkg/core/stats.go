package core

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"

	g "maragu.dev/gomponents"
	. "maragu.dev/gomponents/html"
)

// StatsContent component for the /stats page
func StatsContent(platformCounts map[string]int, statsErr error,
	assetCounts map[string]int, assetErr error) g.Node {

	content := []g.Node{
		H1(Class("text-3xl md:text-4xl font-bold text-gray-800 mb-8"), g.Text("Program Statistics")),
	}

	if statsErr != nil {
		content = append(content,
			Div(Class("bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded relative mb-4"),
				Strong(g.Text("Error loading stats: ")),
				g.Text(statsErr.Error()),
			),
		)
	}

	if assetErr != nil {
		content = append(content,
			Div(Class("bg-orange-100 border border-orange-400 text-orange-700 px-4 py-3 rounded relative mb-4"),
				Strong(g.Text("Asset Statistics Information: ")),
				g.Text(assetErr.Error()),
			),
		)
	}

	h1Count := platformCounts["hackerone"]
	bcCount := platformCounts["bugcrowd"]
	ywhCount := platformCounts["yeswehack"]
	itCount := platformCounts["intigriti"]

	// Bar Chart for Program Counts
	maxProgramCount := h1Count
	if bcCount > maxProgramCount {
		maxProgramCount = bcCount
	}
	if ywhCount > maxProgramCount {
		maxProgramCount = ywhCount
	}
	if itCount > maxProgramCount {
		maxProgramCount = itCount
	}
	if maxProgramCount == 0 {
		maxProgramCount = 1
	}

	maxBarHeightPx := 200

	h1BarHeight := (float64(h1Count) / float64(maxProgramCount)) * float64(maxBarHeightPx)
	bcBarHeight := (float64(bcCount) / float64(maxProgramCount)) * float64(maxBarHeightPx)
	ywhBarHeight := (float64(ywhCount) / float64(maxProgramCount)) * float64(maxBarHeightPx)
	itBarHeight := (float64(itCount) / float64(maxProgramCount)) * float64(maxBarHeightPx)

	if h1Count == 0 && bcCount == 0 && ywhCount == 0 && itCount == 0 {
		h1BarHeight, bcBarHeight, ywhBarHeight, itBarHeight = 0, 0, 0, 0
	}

	programChart := Div(Class("mt-8 p-6 bg-gray-50 rounded-lg shadow"),
		H2(Class("text-xl font-semibold text-gray-700 mb-6 text-center"), g.Text("Program Counts by Platform")),
		Div(Class("flex justify-around items-end space-x-2 md:space-x-4 h-64"),
			Div(Class("flex flex-col items-center w-1/5"),
				Div(Class("text-sm font-medium text-gray-700 mb-1 text-center"), g.Text(fmt.Sprintf("HackerOne (%d)", h1Count))),
				Div(
					Class("w-12 md:w-16 bg-blue-500 rounded-t-md hover:bg-blue-600 transition-colors"),
					g.Attr("style", fmt.Sprintf("height: %.0fpx;", h1BarHeight)),
				),
			),
			Div(Class("flex flex-col items-center w-1/5"),
				Div(Class("text-sm font-medium text-gray-700 mb-1 text-center"), g.Text(fmt.Sprintf("Bugcrowd (%d)", bcCount))),
				Div(
					Class("w-12 md:w-16 bg-green-500 rounded-t-md hover:bg-green-600 transition-colors"),
					g.Attr("style", fmt.Sprintf("height: %.0fpx;", bcBarHeight)),
				),
			),
			Div(Class("flex flex-col items-center w-1/5"),
				Div(Class("text-sm font-medium text-gray-700 mb-1 text-center"), g.Text(fmt.Sprintf("YesWeHack (%d)", ywhCount))),
				Div(
					Class("w-12 md:w-16 bg-yellow-500 rounded-t-md hover:bg-yellow-600 transition-colors"),
					g.Attr("style", fmt.Sprintf("height: %.0fpx;", ywhBarHeight)),
				),
			),
			Div(Class("flex flex-col items-center w-1/5"),
				Div(Class("text-sm font-medium text-gray-700 mb-1 text-center"), g.Text(fmt.Sprintf("Intigriti (%d)", itCount))),
				Div(
					Class("w-12 md:w-16 bg-purple-500 rounded-t-md hover:bg-purple-600 transition-colors"),
					g.Attr("style", fmt.Sprintf("height: %.0fpx;", itBarHeight)),
				),
			),
		),
	)

	if statsErr == nil {
		content = append(content, programChart)
	} else {
		content = append(content, P(Class("text-lg text-gray-600 mt-8"), g.Text("Could not load any program statistics data to display chart.")))
	}

	// Asset Counts by Type Bar Chart
	if len(assetCounts) > 0 {
		type assetStatData struct {
			Category string
			Count    int
		}
		var sortedAssetStats []assetStatData
		for cat, count := range assetCounts {
			sortedAssetStats = append(sortedAssetStats, assetStatData{Category: cat, Count: count})
		}
		sort.Slice(sortedAssetStats, func(i, j int) bool {
			if sortedAssetStats[i].Count != sortedAssetStats[j].Count {
				return sortedAssetStats[i].Count > sortedAssetStats[j].Count
			}
			return sortedAssetStats[i].Category < sortedAssetStats[j].Category
		})

		maxAssetCountForChart := 0
		if len(sortedAssetStats) > 0 {
			for _, s := range sortedAssetStats {
				if s.Count > maxAssetCountForChart {
					maxAssetCountForChart = s.Count
				}
			}
		}
		if maxAssetCountForChart == 0 {
			maxAssetCountForChart = 1
		}

		assetChartBars := []g.Node{}
		const barColorClass = "bg-blue-500"
		const barHoverColorClass = "hover:bg-blue-600"

		for _, stat := range sortedAssetStats {
			barPercentage := (float64(stat.Count) / float64(maxAssetCountForChart)) * 100.0
			if stat.Count == 0 {
				barPercentage = 0
			}

			assetChartBars = append(assetChartBars,
				Div(Class("flex items-center my-1.5"),
					Div(Class("w-1/3 md:w-1/4 shrink-0 text-xs font-medium text-gray-600 text-right pr-3 break-words"),
						g.Text(fmt.Sprintf("%s (%d)", stat.Category, stat.Count)),
					),
					Div(Class("flex-grow bg-gray-200 rounded-md h-5 md:h-6"),
						Div(
							Class(fmt.Sprintf("%s %s rounded-md h-5 md:h-6 transition-colors", barColorClass, barHoverColorClass)),
							g.Attr("style", fmt.Sprintf("width: %.2f%%;", barPercentage)),
							g.Attr("title", fmt.Sprintf("%s: %d assets", stat.Category, stat.Count)),
						),
					),
				),
			)
		}

		assetTypeChart := Div(Class("mt-12 p-6 bg-gray-50 rounded-lg shadow"),
			H2(Class("text-xl font-semibold text-gray-700 mb-6 text-center"), g.Text("In-Scope Assets by Type")),
			Div(Class("max-h-[32rem] overflow-y-auto pr-2"),
				g.Group(assetChartBars),
			),
		)
		content = append(content, assetTypeChart)

	} else {
		if assetErr == nil {
			content = append(content, P(Class("text-lg text-gray-600 mt-8"), g.Text("No in-scope asset data found to display by type.")))
		} else {
			content = append(content, P(Class("text-lg text-gray-600 mt-8"), g.Text("Could not generate asset type statistics chart due to loading errors or no data.")))
		}
	}

	return Main(Class("container mx-auto mt-8 mb-16 p-4"),
		Section(Class("md:bg-white md:rounded-lg md:shadow-xl md:p-8 lg:p-12"),
			g.Group(content),
		),
	)
}

// statsHandler handles requests for the /stats page.
func statsHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/stats" {
		http.NotFound(w, r)
		return
	}

	ctx := context.Background()

	// Get platform stats from DB
	platformCounts := make(map[string]int)
	var statsErr error
	stats, err := db.GetStats(ctx)
	if err != nil {
		statsErr = err
		log.Printf("Error getting platform stats: %v", err)
	} else {
		for _, s := range stats {
			platformCounts[strings.ToLower(s.Platform)] = s.ProgramCount
		}
	}

	// Count assets by category from scope entries
	assetCounts := make(map[string]int)
	var assetErr error
	allEntries, err := loadScopeFromDB()
	if err != nil {
		assetErr = err
		log.Printf("Error getting asset counts by type: %v", err)
	} else {
		for _, entry := range allEntries {
			if !strings.HasSuffix(entry.Element, " (OOS)") {
				if entry.Category != "" {
					assetCounts[entry.Category]++
				}
			}
		}
	}

	PageLayout(
		"Platform statistics - bbscope.com",
		"View statistics and analytics for bug bounty programs across different platforms. Compare program counts from HackerOne, Bugcrowd, YesWeHack, Intigriti and other security platforms.",
		Navbar(),
		StatsContent(platformCounts, statsErr, assetCounts, assetErr),
		FooterEl(),
		"",
		false,
	).Render(w)
}
