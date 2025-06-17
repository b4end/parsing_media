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
	lentaURL        = "https://lenta.ru"
	lentaURLPage    = "https://lenta.ru/parts/news/"
	numWorkersLenta = 10
)

func LentaMain() {
	totalStartTime := time.Now()
	_ = getLinksLenta()
	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("%s[LENTA]%s[INFO] Парсер Lenta.ru заверщил работу: (%s)%s\n", ColorBlue, ColorYellow, FormatDuration(totalElapsedTime), ColorReset)
}

func getLinksLenta() []Data {
	var foundLinks []string
	seenLinks := make(map[string]bool)

	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	doc, err := GetHTMLForClient(client, lentaURLPage)
	if err != nil {
		fmt.Printf("%s[LENTA]%s[ERROR] Ошибка при получении HTML со страницы %s: %v%s\n", ColorBlue, ColorRed, lentaURLPage, err, ColorReset)
		return getPageLenta(foundLinks)
	}

	linkSelector := "a.card-full-news._parts-news"
	doc.Find(linkSelector).Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists {
			fullHref := ""
			if strings.HasPrefix(href, "/") {
				fullHref = lentaURL + href
			} else if strings.HasPrefix(href, lentaURL) {
				fullHref = href
			}

			if fullHref != "" && !seenLinks[fullHref] {
				seenLinks[fullHref] = true
				foundLinks = append(foundLinks, fullHref)
			}
		}
	})

	if len(foundLinks) == 0 {
		fmt.Printf("%s[LENTA]%s[WARNING] Не найдено ссылок с селектором '%s' на странице %s.%s\n", ColorBlue, ColorYellow, linkSelector, lentaURLPage, ColorReset)
	}

	return getPageLenta(foundLinks)
}

type pageParseResultLenta struct {
	Data    Data
	Error   error
	PageURL string
	IsEmpty bool
	Reasons []string
}

func getPageLenta(links []string) []Data {
	var products []Data
	var errItems []string
	totalLinks := len(links)

	if totalLinks == 0 {
		return products
	}

	locationPlus3 := time.FixedZone("UTC+3", 3*60*60)
	dateLayout := "15:04, 2 01 2006"
	tagsAreMandatory := true

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: numWorkersLenta + 5,
			IdleConnTimeout:     90 * time.Second,
			MaxConnsPerHost:     numWorkersLenta,
		},
	}

	resultsChan := make(chan pageParseResultLenta, totalLinks)
	linkChan := make(chan string, totalLinks)

	for _, link := range links {
		linkChan <- link
	}
	close(linkChan)

	var wg sync.WaitGroup

	actualNumWorkers := numWorkersLenta
	if totalLinks < numWorkersLenta {
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

				doc, err := GetHTMLForClient(httpClient, pageURL)
				if err != nil {
					resultsChan <- pageParseResultLenta{PageURL: pageURL, Error: fmt.Errorf("ошибка GET: %w", err)}
					continue
				}

				title = strings.TrimSpace(doc.Find(".topic-body__title").First().Text())

				var bodyBuilder strings.Builder
				doc.Find(".topic-body__content > p").Each(func(i int, s *goquery.Selection) {
					paragraphText := strings.TrimSpace(s.Text())
					if paragraphText != "" {
						if bodyBuilder.Len() > 0 {
							bodyBuilder.WriteString("\n\n")
						}
						bodyBuilder.WriteString(paragraphText)
					}
				})
				body = bodyBuilder.String()

				dateTextRaw := doc.Find("a.topic-header__item.topic-header__time").First().Text()
				dateToParse := strings.TrimSpace(dateTextRaw)
				processedDateStr := dateToParse
				var dateParseError error

				if dateToParse != "" {
					foundMonth := false
					lowerDateToParse := strings.ToLower(dateToParse)
					tempProcessedStr := dateToParse

					for rusMonth, numMonth := range RussianMonths {
						lowerRusMonth := strings.ToLower(rusMonth)
						if strings.Contains(lowerDateToParse, lowerRusMonth) {
							startIndex := strings.Index(lowerDateToParse, lowerRusMonth)
							if startIndex != -1 {
								tempProcessedStr = dateToParse[:startIndex] + numMonth + dateToParse[startIndex+len(rusMonth):]
								foundMonth = true
								break
							}
						}
					}
					if foundMonth {
						processedDateStr = tempProcessedStr
					}

					parsedTime, parseErr := time.ParseInLocation(dateLayout, processedDateStr, locationPlus3)
					if parseErr != nil {
						dateParseError = parseErr
						fmt.Printf("%s[LENTA]%s[WARNING] Ошибка парсинга даты: '%s' (попытка с '%s') на %s: %v%s\n", ColorBlue, ColorYellow, dateToParse, processedDateStr, pageURL, parseErr, ColorReset)
					} else {
						parsDate = parsedTime
					}
				}

				doc.Find("a.topic-header__item.topic-header__rubric").Each(func(_ int, s *goquery.Selection) {
					tagText := strings.TrimSpace(s.Text())
					if tagText != "" {
						tags = append(tags, tagText)
					}
				})

				if title != "" && body != "" && !parsDate.IsZero() && (!tagsAreMandatory || len(tags) > 0) {
					dataItem := Data{
						Site:  lentaURL,
						Href:  pageURL,
						Title: title,
						Body:  body,
						Date:  parsDate,
						Tags:  tags,
					}
					hash, err := dataItem.Hashing()
					if err != nil {
						resultsChan <- pageParseResultLenta{PageURL: pageURL, Error: fmt.Errorf("ошибка генерации хеша: %w", err)}
						continue
					}
					dataItem.Hash = hash
					resultsChan <- pageParseResultLenta{Data: dataItem}
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
							reasonDate = fmt.Sprintf("D:false (err: %v, original_str: '%s', processed_str: '%s')", dateParseError, dateToParse, processedDateStr)
						} else if dateToParse == "" {
							reasonDate = "D:false (empty_str)"
						}
						reasons = append(reasons, reasonDate)
					}
					if tagsAreMandatory && len(tags) == 0 {
						reasons = append(reasons, "Tags:false")
					}
					resultsChan <- pageParseResultLenta{PageURL: pageURL, IsEmpty: true, Reasons: reasons}
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	for result := range resultsChan {
		if result.Error != nil {
			errItems = append(errItems, fmt.Sprintf("%s (%s)", result.PageURL, result.Error.Error()))
		} else if result.IsEmpty {
			errItems = append(errItems, fmt.Sprintf("%s (нет данных: %s)", result.PageURL, strings.Join(result.Reasons, ", ")))
		} else {
			products = append(products, result.Data)
		}
	}

	if len(products) > 0 {
		if len(errItems) > 0 {
			fmt.Printf("%s[LENTA]%s[WARNING] Не удалось обработать %d из %d страниц (или отсутствовали данные):%s\n", ColorBlue, ColorYellow, len(errItems), totalLinks, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	} else if totalLinks > 0 {
		fmt.Printf("%s[LENTA]%s[ERROR] Парсинг статей Lenta.ru завершен, но не удалось собрать данные ни с одной из %d страниц.%s\n", ColorBlue, ColorRed, totalLinks, ColorReset)
		if len(errItems) > 0 {
			fmt.Printf("%s[LENTA]%s[INFO] Список страниц с ошибками или без данных:%s\n", ColorBlue, ColorYellow, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	}

	return products
}
