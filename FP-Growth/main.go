package main

import (
	"database/sql"
	_ "encoding/json"
	"fmt"
	"log"
	"os"
	_ "os"
	"sort"
	_ "strconv"
	"strings"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type FPNode struct {
	item     string
	count    int
	parent   *FPNode
	children map[string]*FPNode
}

type Rule struct {
	Book_idsR  []int   `json:"result"`
	Book_idsL  []int   `json:"base"`
	Support    int     `json:"support"`
	Confidence float64 `json:"confidence"`
}

func NewFPNode(item string) *FPNode {
	return &FPNode{
		item:     item,
		count:    1,
		children: make(map[string]*FPNode),
	}
}

func insertTree(transaction []string, node *FPNode, headerTable map[string][]*FPNode) {
	currentNode := node
	for _, item := range transaction {
		if child, ok := currentNode.children[item]; ok {
			child.count++
			currentNode = child
		} else {
			newNode := NewFPNode(item)
			newNode.parent = currentNode
			currentNode.children[item] = newNode
			currentNode = newNode
			headerTable[item] = append(headerTable[item], newNode)
		}
	}
}

func mineTree(headerTable map[string][]*FPNode, minSupport int, prefix []string) map[string]int {
	frequentPatterns := make(map[string]int)
	for item, nodes := range headerTable {
		support := 0
		for _, node := range nodes {
			support += node.count
		}
		if support >= minSupport {
			newPrefix := append([]string{item}, prefix...)
			frequentPatterns[fmt.Sprintf("%v", newPrefix)] = support

			conditionalPatternBase := [][]string{}
			for _, node := range nodes {
				pattern := []string{}
				parent := node.parent
				for parent.item != "null" {
					pattern = append([]string{parent.item}, pattern...)
					parent = parent.parent
				}
				for i := 0; i < node.count; i++ {
					conditionalPatternBase = append(conditionalPatternBase, pattern)
				}
			}

			conditionalHeaderTable := make(map[string][]*FPNode)
			conditionalRoot := NewFPNode("null")
			for _, pattern := range conditionalPatternBase {
				insertTree(pattern, conditionalRoot, conditionalHeaderTable)
			}

			subPatterns := mineTree(conditionalHeaderTable, minSupport, newPrefix)
			for k, v := range subPatterns {
				frequentPatterns[k] = v
			}
		}
	}
	return frequentPatterns
}

func generateRules(frequentPatterns map[string]int, confidenceThreshold float64) {
	// rules := []Rule{}
	counter := 0
	for pattern, support := range frequentPatterns {
		items := strings.Split(pattern[1:len(pattern)-1], " ")
		if len(items) > 1 {
			for i := 1; i < len(items); i++ {
				subset := items[:i]
				remaining := items[i:]

				subsetSupport := frequentPatterns[fmt.Sprintf("%v", subset)]
				confidence := float64(support) / float64(subsetSupport) * 100

				if confidence >= confidenceThreshold {
					fmt.Printf("Rule: %v => %v, Support: %d, Confidence: %.2f%%\n", subset, remaining, support, confidence)
					counter++
					// rule := Rule{
					// 	Support:    support,
					// 	Confidence: confidence,
					// 	Book_idsR:  []int{},
					// 	Book_idsL:  []int{},
					// }
					// for _, val := range subset {
					// 	x, _ := strconv.Atoi(val)
					// 	rule.Book_idsL = append(rule.Book_idsL, x)
					// }
					// for _, val := range remaining {
					// 	x, _ := strconv.Atoi(val)
					// 	rule.Book_idsR = append(rule.Book_idsR, x)
					// }
					//rules = append(rules, rule)
				}
			}
		}
	}
	log.Println("Rules generated: ", counter)
	// jsonData, err := json.MarshalIndent(rules, "", "  ")
	// if err != nil {
	// 	fmt.Println("Error marshaling data:", err)
	// 	return
	// }

	// // Step 2: Write the JSON data to a file
	// file, err := os.Create("rules.json")
	// if err != nil {
	// 	fmt.Println("Error creating file:", err)
	// 	return
	// }
	// defer file.Close()
	// _, err = file.Write(jsonData)
	// if err != nil {
	// 	fmt.Println("Error writing to file:", err)
	// 	return
	// }
	// fmt.Println("Data successfully written to rules.json")
}

func main() {
	err := godotenv.Load()
	if err != nil {
		fmt.Println("Error loading .env file")
	}
	port := os.Getenv("DB_PORT")
	database := os.Getenv("DB_DB")
	user := os.Getenv("DB_USER")
	pass := os.Getenv("DB_PASSWORD")
	host := os.Getenv("DB_HOST")
	db, err := sql.Open("postgres", fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable", host, user, pass, database, port))
	if err != nil {
		panic(err.Error())
	}
	data, _ := db.Query("SELECT * FROM user_read")
	alldata := make(map[string][]string)
	for data.Next() {
		var bid string
		var uid string
		if err = data.Scan(&bid, &uid); err != nil {
			panic(err)
		}
		_, ok := alldata[uid]
		if ok {
			newdata := append(alldata[uid], bid)
			alldata[uid] = newdata
		} else {
			alldata[uid] = []string{bid}
		}
	}
	log.Println(len(alldata))
	//count item-freq
	itemCount := make(map[string]int)
	for _, transaction := range alldata {
		for _, item := range transaction {
			itemCount[item]++
		}
	}

	//sort items by freq
	for _, transaction := range alldata {
		sort.SliceStable(transaction, func(i, j int) bool {
			return itemCount[transaction[i]] > itemCount[transaction[j]]
		})
	}

	//FP tree builder
	root := NewFPNode("null")
	headerTable := make(map[string][]*FPNode)
	for _, transaction := range alldata {
		insertTree(transaction, root, headerTable)
	}
	//patterns with min sup
	minSupport := 10
	frequentPatterns := mineTree(headerTable, minSupport, []string{})

	//generating all rules with given conf
	confidenceThreshold := 60.0
	generateRules(frequentPatterns, confidenceThreshold)
}
