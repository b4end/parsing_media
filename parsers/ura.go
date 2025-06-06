package parsers

import (
	"fmt"
	"net/http"
	. "parsing_media/utils"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	uraURL        = "https://ura.news"
	numWorkersUra = 10
)

func UraMain() {
	totalStartTime := time.Now()

	_ = getLinksUra()

	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("%s[URA]%s[INFO] Парсер URA.RU завершил работу: (%s)%s\n", ColorBlue, ColorYellow, FormatDuration(totalElapsedTime), ColorReset)
}

func getLinksUra() []Data {
	var foundLinks []string
	seenLinks := make(map[string]bool)
	linkSelector := "ul li.list-scroll-item > a"

	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	doc, err := GetHTMLForClient(client, uraURL)
	if err != nil {
		fmt.Printf("%s[URA]%s[ERROR] Ошибка при получении HTML со страницы %s: %v%s\n", ColorBlue, ColorRed, uraURL, err, ColorReset)
		return getPageUra(foundLinks)
	}

	doc.Find(linkSelector).Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists {
			if strings.HasPrefix(href, "/news/") {
				fullLink := uraURL + href
				if !seenLinks[fullLink] {
					seenLinks[fullLink] = true
					foundLinks = append(foundLinks, fullLink)
				}
			}
		}
	})

	if len(foundLinks) == 0 {
		fmt.Printf("%s[URA]%s[WARNING] Не найдено ссылок с селектором '%s' на странице %s.%s\n", ColorBlue, ColorYellow, linkSelector, uraURL, ColorReset)
	}

	return getPageUra(foundLinks)
}

type pageParseResultUra struct {
	Data    Data
	Error   error
	PageURL string
	IsEmpty bool
	Reasons []string
}

func getPageUra(links []string) []Data {
	var products []Data
	var errItems []string
	totalLinks := len(links)

	if totalLinks == 0 {
		return products
	}

	tagsAreMandatory := true

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: numWorkersUra + 5,
			IdleConnTimeout:     90 * time.Second,
			MaxConnsPerHost:     numWorkersUra,
		},
	}

	resultsChan := make(chan pageParseResultUra, totalLinks)
	linkChan := make(chan string, totalLinks)

	for _, link := range links {
		linkChan <- link
	}
	close(linkChan)

	var wg sync.WaitGroup

	actualNumWorkers := numWorkersUra
	if totalLinks < numWorkersUra {
		actualNumWorkers = totalLinks
	}

	targetLocation := time.FixedZone("MSK", 3*60*60)

	for i := 0; i < actualNumWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for pageURL := range linkChan {
				var title, body string
				var tags []string
				var parsDate time.Time
				var dateParseError error
				var dateStringRaw string

				doc, err := GetHTMLForClient(httpClient, pageURL)
				if err != nil {
					resultsChan <- pageParseResultUra{PageURL: pageURL, Error: fmt.Errorf("ошибка GET: %w", err)}
					continue
				}

				title = strings.TrimSpace(doc.Find("h1.publication-title").First().Text())

				var bodyBuilder strings.Builder
				articleBodyNode := doc.Find("div.item-text[itemprop='articleBody']")
				if articleBodyNode.Length() > 0 {
					articleBodyNode.Find(".item-text-incut, .inpage_block-adv-c, .custom-html, .publication-send-news, .yandex-rss-hidden").Remove()

					articleBodyNode.Find("p").Each(func(j int, s *goquery.Selection) {
						currentTextPart := strings.TrimSpace(s.Text())
						if currentTextPart != "" {
							if bodyBuilder.Len() > 0 {
								bodyBuilder.WriteString("\n\n")
							}
							bodyBuilder.WriteString(currentTextPart)
						}
					})
				}
				body = bodyBuilder.String()

				dateStringRaw = doc.Find("time.time2[itemprop='datePublished']").AttrOr("datetime", "")
				if dateStringRaw != "" {
					parsedTime, parseErr := time.Parse(time.RFC3339, dateStringRaw)
					if parseErr != nil {
						dateParseError = parseErr
						fmt.Printf("%s[URA]%s[WARNING] Ошибка парсинга даты: '%s' на %s: %v%s\n", ColorBlue, ColorYellow, dateStringRaw, pageURL, parseErr, ColorReset)
					} else {
						parsDate = parsedTime.In(targetLocation)
					}
				} else {
					dateParseError = fmt.Errorf("атрибут datetime не найден или пуст")
					fmt.Printf("%s[URA]%s[WARNING] Атрибут datetime для даты не найден на %s%s\n", ColorBlue, ColorYellow, pageURL, ColorReset)
				}

				doc.Find("div.publication-rubrics-container a span[itemprop='name']").Each(func(_ int, s *goquery.Selection) {
					tagText := strings.TrimSpace(s.Text())
					if tagText != "" {
						tags = append(tags, tagText)
					}
				})

				if title != "" && body != "" && !parsDate.IsZero() && (!tagsAreMandatory || len(tags) > 0) {
					resultsChan <- pageParseResultUra{Data: Data{
						Href:  pageURL,
						Title: title,
						Body:  body,
						Date:  parsDate,
						Tags:  tags,
					}}
				} else {
					var reasons []string
					if title == "" {
						reasons = append(reasons, "T:false")
					}
					if body == "" {
						reasons = append(reasons, "B:false")
					}
					if parsDate.IsZero() {
						reasonDate := "D:false"
						if dateParseError != nil {
							reasonDate = fmt.Sprintf("D:false (err: %v, str: '%s')", dateParseError, dateStringRaw)
						} else if dateStringRaw == "" {
							reasonDate = "D:false (атрибут datetime не найден)"
						}
						reasons = append(reasons, reasonDate)
					}
					if tagsAreMandatory && len(tags) == 0 {
						reasons = append(reasons, "Tags:false")
					}
					resultsChan <- pageParseResultUra{PageURL: pageURL, IsEmpty: true, Reasons: reasons}
				}
			}
		}(i)
	}

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	processedCount := 0
	for result := range resultsChan {
		processedCount++
		if result.Error != nil {
			errItems = append(errItems, fmt.Sprintf("%s (%s)", result.PageURL, result.Error.Error()))
		} else if result.IsEmpty {
			errItems = append(errItems, fmt.Sprintf("%s (нет данных: %s)", result.PageURL, strings.Join(result.Reasons, ", ")))
		} else {
			products = append(products, result.Data)
		}
	}

	if len(errItems) > 0 {
		fmt.Printf("%s[URA]%s[WARNING] Не удалось обработать %d из %d страниц (или отсутствовали данные):%s\n", ColorBlue, ColorYellow, len(errItems), totalLinks, ColorReset)
		maxErrorsToShow := 20
		for idx, itemMessage := range errItems {
			if idx < maxErrorsToShow {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			} else if idx == maxErrorsToShow {
				fmt.Printf("%s  ... и еще %d ошибок/предупреждений ...%s\n", ColorYellow, len(errItems)-maxErrorsToShow, ColorReset)
				break
			}
		}
	}

	if len(products) == 0 && totalLinks > 0 {
		fmt.Printf("%s[URA]%s[ERROR] Парсинг статей URA.RU завершен, но не удалось собрать данные ни с одной из %d страниц.%s\n", ColorBlue, ColorRed, totalLinks, ColorReset)
	}
	return products
}
