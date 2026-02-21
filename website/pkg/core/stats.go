package core

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"

	g "maragu.dev/gomponents"
	. "maragu.dev/gomponents/html"
)

// StatsContent component for the /stats page
func StatsContent(platformCounts map[string]int, statsErr error,
	assetCounts map[string]int, assetErr error) g.Node {

	content := []g.Node{
		H1(Class("text-3xl md:text-4xl font-bold text-slate-100 mb-8"), g.Text("Program Statistics")),
	}

	if statsErr != nil {
		content = append(content,
			Div(Class("bg-red-900/30 border border-red-700 text-red-400 px-4 py-3 rounded relative mb-4"),
				Strong(g.Text("Error loading stats: ")),
				g.Text(statsErr.Error()),
			),
		)
	}

	if assetErr != nil {
		content = append(content,
			Div(Class("bg-orange-900/30 border border-orange-700 text-orange-400 px-4 py-3 rounded relative mb-4"),
				Strong(g.Text("Asset Statistics Information: ")),
				g.Text(assetErr.Error()),
			),
		)
	}

	h1Count := platformCounts["h1"]
	bcCount := platformCounts["bc"]
	ywhCount := platformCounts["ywh"]
	itCount := platformCounts["it"]

	// Chart.js doughnut chart for program counts
	programChart := Div(Class("mt-8 p-6 bg-slate-800/50 border border-slate-700 rounded-lg shadow"),
		H2(Class("text-xl font-semibold text-slate-200 mb-6 text-center"), g.Text("Program Counts by Platform")),
		Div(Class("max-w-md mx-auto"),
			Canvas(ID("programChart"), g.Attr("height", "300")),
		),
	)

	if statsErr == nil {
		content = append(content, programChart)
	} else {
		content = append(content, P(Class("text-lg text-slate-400 mt-8"), g.Text("Could not load any program statistics data to display chart.")))
	}

	// Chart.js horizontal bar chart for asset types
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

		// Build JS arrays for Chart.js
		var labels, counts []string
		for _, stat := range sortedAssetStats {
			labels = append(labels, "'"+stat.Category+"'")
			counts = append(counts, strconv.Itoa(stat.Count))
		}

		chartHeight := 30*len(sortedAssetStats) + 100
		if chartHeight < 200 {
			chartHeight = 200
		}

		assetTypeChart := Div(Class("mt-12 p-6 bg-slate-800/50 border border-slate-700 rounded-lg shadow"),
			H2(Class("text-xl font-semibold text-slate-200 mb-6 text-center"), g.Text("In-Scope Assets by Type")),
			Div(Class("max-w-2xl mx-auto"),
				Canvas(ID("assetChart"), g.Attr("height", strconv.Itoa(chartHeight))),
			),
		)
		content = append(content, assetTypeChart)

		// Chart.js initialization scripts (placed after canvases)
		content = append(content,
			Script(Src("https://cdn.jsdelivr.net/npm/chart.js@4/dist/chart.umd.min.js")),
			Script(g.Raw(fmt.Sprintf(`
				new Chart(document.getElementById('programChart'), {
					type: 'doughnut',
					data: {
						labels: ['HackerOne', 'Bugcrowd', 'YesWeHack', 'Intigriti'],
						datasets: [{
							data: [%d, %d, %d, %d],
							backgroundColor: ['#3b82f6', '#10b981', '#eab308', '#8b5cf6'],
							borderColor: '#1e293b',
							borderWidth: 2,
							hoverOffset: 8
						}]
					},
					options: {
						responsive: true,
						plugins: {
							legend: {
								position: 'bottom',
								labels: { color: '#94a3b8', padding: 16, font: { size: 13 } }
							},
							tooltip: {
								backgroundColor: '#1e293b',
								titleColor: '#e2e8f0',
								bodyColor: '#e2e8f0',
								borderColor: '#334155',
								borderWidth: 1
							}
						}
					}
				});
				new Chart(document.getElementById('assetChart'), {
					type: 'bar',
					data: {
						labels: [%s],
						datasets: [{
							label: 'Asset Count',
							data: [%s],
							backgroundColor: '#06b6d4',
							borderColor: '#0891b2',
							borderWidth: 1,
							borderRadius: 4
						}]
					},
					options: {
						indexAxis: 'y',
						responsive: true,
						scales: {
							x: { ticks: { color: '#94a3b8' }, grid: { color: '#1e293b' } },
							y: { ticks: { color: '#94a3b8' }, grid: { color: '#1e293b' } }
						},
						plugins: {
							legend: { display: false },
							tooltip: {
								backgroundColor: '#1e293b',
								titleColor: '#e2e8f0',
								bodyColor: '#e2e8f0',
								borderColor: '#334155',
								borderWidth: 1
							}
						}
					}
				});
			`, h1Count, bcCount, ywhCount, itCount,
				strings.Join(labels, ","),
				strings.Join(counts, ","),
			))),
		)
	} else {
		if assetErr == nil {
			content = append(content, P(Class("text-lg text-slate-400 mt-8"), g.Text("No in-scope asset data found to display by type.")))
		} else {
			content = append(content, P(Class("text-lg text-slate-400 mt-8"), g.Text("Could not generate asset type statistics chart due to loading errors or no data.")))
		}

		// If there are no asset counts but platform chart exists, still init Chart.js for it
		if statsErr == nil {
			content = append(content,
				Script(Src("https://cdn.jsdelivr.net/npm/chart.js@4/dist/chart.umd.min.js")),
				Script(g.Raw(fmt.Sprintf(`
					new Chart(document.getElementById('programChart'), {
						type: 'doughnut',
						data: {
							labels: ['HackerOne', 'Bugcrowd', 'YesWeHack', 'Intigriti'],
							datasets: [{
								data: [%d, %d, %d, %d],
								backgroundColor: ['#3b82f6', '#10b981', '#eab308', '#8b5cf6'],
								borderColor: '#1e293b',
								borderWidth: 2,
								hoverOffset: 8
							}]
						},
						options: {
							responsive: true,
							plugins: {
								legend: {
									position: 'bottom',
									labels: { color: '#94a3b8', padding: 16, font: { size: 13 } }
								},
								tooltip: {
									backgroundColor: '#1e293b',
									titleColor: '#e2e8f0',
									bodyColor: '#e2e8f0',
									borderColor: '#334155',
									borderWidth: 1
								}
							}
						}
					});
				`, h1Count, bcCount, ywhCount, itCount))),
			)
		}
	}

	return Main(Class("container mx-auto mt-8 mb-16 p-4"),
		Section(Class("bg-slate-900/50 border border-slate-800 rounded-lg shadow-xl p-6 md:p-8 lg:p-12"),
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

	// Count assets by category from DB
	var assetErr error
	assetCounts, err := db.GetAssetCountsByCategory(ctx)
	if err != nil {
		assetErr = err
		assetCounts = make(map[string]int)
		log.Printf("Error getting asset counts by type: %v", err)
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
