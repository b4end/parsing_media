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
	lifeURL         = "https://life.ru"
	lifeNewsPageURL = "https://life.ru/s/novosti/last"
	numWorkersLife  = 10
)

func LifeMain() {
	totalStartTime := time.Now()
	articles, links := getLinksLife()
	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("%s[LIFE]%s[INFO] Парсер Life.ru заверщил работу собрав (%d/%d): (%s)%s\n", ColorBlue, ColorYellow, len(articles), len(links), FormatDuration(totalElapsedTime), ColorReset)
}

func getLinksLife() ([]Data, []string) {
	var foundLinks []string
	seenLinks := make(map[string]bool)
	linkSelector := "div.styles_postsList__MBykd a.styles_root__2aHN8"

	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	doc, err := GetHTMLForClient(client, lifeNewsPageURL)
	if err != nil {
		fmt.Printf("%s[LIFE]%s[ERROR] Ошибка при получении HTML со страницы %s: %v%s\n", ColorBlue, ColorRed, lifeNewsPageURL, err, ColorReset)
		return getPageLife(foundLinks)
	}

	doc.Find(linkSelector).Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists {
			if strings.HasPrefix(href, "/p/") {
				fullLink := lifeURL + href
				if !seenLinks[fullLink] {
					seenLinks[fullLink] = true
					foundLinks = append(foundLinks, fullLink)
				}
			}
		}
	})

	if len(foundLinks) == 0 {
		fmt.Printf("%s[LIFE]%s[WARNING] Не найдено ссылок с селектором '%s' на странице %s.%s\n", ColorBlue, ColorYellow, linkSelector, lifeNewsPageURL, ColorReset)
	}

	return getPageLife(foundLinks)
}

type pageParseResultLife struct {
	Data    Data
	Error   error
	PageURL string
	IsEmpty bool
	Reasons []string
}

func getPageLife(links []string) ([]Data, []string) {
	var products []Data
	var errItems []string
	totalLinks := len(links)

	if totalLinks == 0 {
		return products, links
	}

	tagsAreMandatory := true
	locationMSK := time.FixedZone("MSK", 3*60*60)

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: numWorkersLife + 5,
			IdleConnTimeout:     90 * time.Second,
			MaxConnsPerHost:     numWorkersLife,
		},
	}

	resultsChan := make(chan pageParseResultLife, totalLinks)
	linkChan := make(chan string, totalLinks)

	for _, link := range links {
		linkChan <- link
	}
	close(linkChan)

	var wg sync.WaitGroup

	actualNumWorkers := numWorkersLife
	if totalLinks < numWorkersLife {
		actualNumWorkers = totalLinks
	}

	for i := 0; i < actualNumWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for pageURL := range linkChan {
				var title, body string
				var tags []string
				var parsDate time.Time
				var dateParseError error

				doc, err := GetHTMLForClient(httpClient, pageURL)
				if err != nil {
					resultsChan <- pageParseResultLife{PageURL: pageURL, Error: fmt.Errorf("ошибка GET: %w", err)}
					continue
				}

				title = strings.TrimSpace(doc.Find("h1.styles_title__1Tc08").First().Text())

				var bodyBuilder strings.Builder
				doc.Find("div.indentRules_block__iwiZV.styles_text__3IVkI").Each(func(idx int, textBlock *goquery.Selection) {
					textBlock.Find("p").Each(func(j int, pSelection *goquery.Selection) {
						currentTextPart := strings.TrimSpace(pSelection.Text())
						if currentTextPart != "" {
							if bodyBuilder.Len() > 0 {
								bodyBuilder.WriteString("\n\n")
							}
							bodyBuilder.WriteString(currentTextPart)
						}
					})
				})
				body = bodyBuilder.String()

				dateTextRaw := doc.Find("div.styles_metaItem__1aUkA.styles_smallFont__2p4_v").First().Text()
				dateToParse := strings.TrimSpace(dateTextRaw)

				now := time.Now().In(locationMSK)

				if strings.Contains(dateToParse, "сегодня в") {
					timeStr := strings.Replace(dateToParse, "сегодня в ", "", 1)
					fullDateStr := fmt.Sprintf("%02d.%02d.%d %s", now.Day(), int(now.Month()), now.Year(), timeStr)
					parsedTime, parseErr := time.ParseInLocation("02.01.2006 15:04", fullDateStr, locationMSK)
					if parseErr != nil {
						dateParseError = fmt.Errorf("ошибка парсинга 'сегодня': %w, строка: '%s'", parseErr, fullDateStr)
					} else {
						parsDate = parsedTime
					}
				} else if strings.Contains(dateToParse, "вчера в") {
					yesterday := now.AddDate(0, 0, -1)
					timeStr := strings.Replace(dateToParse, "вчера в ", "", 1)
					fullDateStr := fmt.Sprintf("%02d.%02d.%d %s", yesterday.Day(), int(yesterday.Month()), yesterday.Year(), timeStr)
					parsedTime, parseErr := time.ParseInLocation("02.01.2006 15:04", fullDateStr, locationMSK)
					if parseErr != nil {
						dateParseError = fmt.Errorf("ошибка парсинга 'вчера': %w, строка: '%s'", parseErr, fullDateStr)
					} else {
						parsDate = parsedTime
					}
				} else if dateToParse != "" {
					parts := strings.Split(dateToParse, ",")
					if len(parts) == 2 {
						dayMonthPart := strings.TrimSpace(parts[0])
						timePart := strings.TrimSpace(parts[1])
						dayMonthParts := strings.Fields(dayMonthPart)
						if len(dayMonthParts) == 2 {
							dayStr := dayMonthParts[0]
							monthRu := dayMonthParts[1]
							monthEn, ok := RussianMonthsLife[strings.ToLower(monthRu)]
							if ok {
								fullDateStrToParse := fmt.Sprintf("%s %s %d %s", dayStr, monthEn, now.Year(), timePart)
								parsedTime, parseErr := time.ParseInLocation("2 January 2006 15:04", fullDateStrToParse, locationMSK)
								if parseErr != nil {
									dateParseError = fmt.Errorf("ошибка парсинга даты '%s' форматом '2 January 2006 15:04': %w", fullDateStrToParse, parseErr)
								} else {
									parsDate = parsedTime
								}
							} else {
								dateParseError = fmt.Errorf("неизвестный русский месяц: '%s' в строке '%s'", monthRu, dateToParse)
							}
						} else {
							dateParseError = fmt.Errorf("не удалось разделить день и месяц из '%s' в строке '%s'", dayMonthPart, dateToParse)
						}
					} else {
						dateParseError = fmt.Errorf("не удалось разделить дату и время по запятой в строке '%s'", dateToParse)
					}
				} else {
					dateParseError = fmt.Errorf("строка с датой пуста")
				}

				if dateParseError != nil && dateToParse != "" {
					fmt.Printf("%s[LIFE]%s[WARNING] Ошибка парсинга даты: '%s' на %s: %v%s\n", ColorBlue, ColorYellow, dateToParse, pageURL, dateParseError, ColorReset)
				}

				doc.Find("div.swiper-wrapper div.swiper-slide li.styles_tagsItem__2LNjk a.styles_tag__1D3vf span").Each(func(_ int, s *goquery.Selection) {
					tagText := strings.TrimSpace(s.Text())
					if tagText != "" {
						tags = append(tags, tagText)
					}
				})

				if title != "" && body != "" && !parsDate.IsZero() && (!tagsAreMandatory || len(tags) > 0) {
					dataItem := Data{
						Site:  lifeURL,
						Href:  pageURL,
						Title: title,
						Body:  body,
						Date:  parsDate,
						Tags:  tags,
					}
					hash, err := dataItem.Hashing()
					if err != nil {
						resultsChan <- pageParseResultLife{PageURL: pageURL, Error: fmt.Errorf("ошибка генерации хеша: %w", err)}
						continue
					}
					dataItem.Hash = hash
					resultsChan <- pageParseResultLife{Data: dataItem}
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
							reasonDate = fmt.Sprintf("D:false (err: %v, str: '%s')", dateParseError, dateToParse)
						} else if dateToParse == "" {
							reasonDate = "D:false (empty_str)"
						}
						reasons = append(reasons, reasonDate)
					}
					if tagsAreMandatory && len(tags) == 0 {
						reasons = append(reasons, "Tags:false")
					}
					resultsChan <- pageParseResultLife{PageURL: pageURL, IsEmpty: true, Reasons: reasons}
				}
			}
		}()
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
		fmt.Printf("%s[LIFE]%s[WARNING] Не удалось обработать %d из %d страниц (или отсутствовали данные):%s\n", ColorBlue, ColorYellow, len(errItems), totalLinks, ColorReset)
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
		fmt.Printf("%s[LIFE]%s[ERROR] Парсинг статей Life.ru завершен, но не удалось собрать данные ни с одной из %d страниц.%s\n", ColorBlue, ColorRed, totalLinks, ColorReset)
	}
	return products, links
}
