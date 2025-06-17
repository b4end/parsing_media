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
	dumatvURL         = "https://dumatv.ru"
	dumatvNewsHTMLURL = "https://dumatv.ru/categories/news"
	numWorkersDumaTV  = 10
)

func DumaTVMain() {
	totalStartTime := time.Now()
	_ = getLinksDumaTV()
	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("%s[DUMATV]%s[INFO] Парсер DumaTV.ru заверщил работу: (%s)%s\n", ColorBlue, ColorYellow, FormatDuration(totalElapsedTime), ColorReset)
}

func getLinksDumaTV() []Data {
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

	doc, err := GetHTMLForClient(client, dumatvNewsHTMLURL)
	if err != nil {
		fmt.Printf("%s[DUMATV]%s[ERROR] Ошибка при получении HTML со страницы %s: %v%s\n", ColorBlue, ColorRed, dumatvNewsHTMLURL, err, ColorReset)
		return getPageDumaTV(foundLinks)
	}

	linkSelector := "div.news-page-list__item a.news-page-card__title"

	doc.Find(linkSelector).Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists {
			fullHref := ""
			if strings.HasPrefix(href, "/") {
				fullHref = dumatvURL + href
			} else if strings.HasPrefix(href, dumatvURL) {
				fullHref = href
			}

			if fullHref != "" && !seenLinks[fullHref] {
				seenLinks[fullHref] = true
				foundLinks = append(foundLinks, fullHref)
			}
		}
	})

	if len(foundLinks) <= 0 {
		fmt.Printf("%s[DUMATV]%s[WARNING] Не найдено ссылок с селектором '%s' на странице %s.%s\n", ColorBlue, ColorYellow, linkSelector, dumatvNewsHTMLURL, ColorReset)
	}

	return getPageDumaTV(foundLinks)
}

type pageParseResultDumaTV struct {
	Data    Data
	Error   error
	PageURL string
	IsEmpty bool
	Reasons []string
}

func getPageDumaTV(links []string) []Data {
	var products []Data
	var errItems []string
	totalLinks := len(links)

	if totalLinks == 0 {
		return products
	}

	locationPlus3 := time.FixedZone("UTC+3", 3*60*60)
	layout := "2 01 2006 / 15:04"
	tagsAreMandatory := true

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: numWorkersDumaTV + 5,
			IdleConnTimeout:     90 * time.Second,
			MaxConnsPerHost:     numWorkersDumaTV,
		},
	}

	resultsChan := make(chan pageParseResultDumaTV, totalLinks)
	linkChan := make(chan string, totalLinks)

	for _, link := range links {
		linkChan <- link
	}
	close(linkChan)

	var wg sync.WaitGroup

	actualNumWorkers := numWorkersDumaTV
	if totalLinks < numWorkersDumaTV {
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
					resultsChan <- pageParseResultDumaTV{PageURL: pageURL, Error: fmt.Errorf("ошибка GET: %w", err)}
					continue
				}

				title = strings.TrimSpace(doc.Find("h1.news-post-content__title").First().Text())

				var bodyBuilder strings.Builder
				doc.Find("div.news-post-content__text").ChildrenFiltered("p, blockquote").Each(func(_ int, s *goquery.Selection) {
					paragraphText := strings.TrimSpace(s.Text())
					if paragraphText != "" {
						if bodyBuilder.Len() > 0 {
							bodyBuilder.WriteString("\n\n")
						}
						bodyBuilder.WriteString(paragraphText)
					}
				})
				body = bodyBuilder.String()

				doc.Find("div.post-tags div.post-tags__item a").Each(func(_ int, s *goquery.Selection) {
					tagText := strings.TrimSpace(s.Text())
					if tagText != "" {
						tags = append(tags, tagText)
					}
				})

				dateTextRaw := doc.Find("div.news-post-top__date").First().Text()
				dateToParse := strings.TrimSpace(dateTextRaw)
				processedStr := dateToParse
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
						processedStr = tempProcessedStr
					}

					parsedTime, parseErr := time.ParseInLocation(layout, processedStr, locationPlus3)
					if parseErr != nil {
						dateParseError = parseErr
						fmt.Printf("%s[DUMATV]%s[WARNING] Ошибка парсинга даты: '%s' (попытка с '%s') на %s: %v%s\n", ColorBlue, ColorYellow, dateToParse, processedStr, pageURL, parseErr, ColorReset)
					} else {
						parsDate = parsedTime
					}
				}

				if title != "" && body != "" && !parsDate.IsZero() && (!tagsAreMandatory || len(tags) != 0) {
					dataItem := Data{
						Site:  dumatvURL,
						Href:  pageURL,
						Title: title,
						Body:  body,
						Date:  parsDate,
						Tags:  tags,
					}
					hash, err := dataItem.Hashing()
					if err != nil {
						resultsChan <- pageParseResultDumaTV{PageURL: pageURL, Error: fmt.Errorf("ошибка генерации хеша: %w", err)}
						continue
					}
					dataItem.Hash = hash
					resultsChan <- pageParseResultDumaTV{Data: dataItem}
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
							reasonDate = fmt.Sprintf("D:false (err: %v, original_str: '%s', processed_str: '%s')", dateParseError, dateToParse, processedStr)
						} else if dateToParse == "" {
							reasonDate = "D:false (empty_str)"
						}
						reasons = append(reasons, reasonDate)
					}
					if tagsAreMandatory && len(tags) == 0 {
						reasons = append(reasons, "Tags:false")
					}
					resultsChan <- pageParseResultDumaTV{PageURL: pageURL, IsEmpty: true, Reasons: reasons}
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
			fmt.Printf("%s[DUMATV]%s[WARNING] Не удалось обработать %d из %d страниц (или отсутствовали данные):%s\n", ColorBlue, ColorYellow, len(errItems), totalLinks, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	} else if totalLinks > 0 {
		fmt.Printf("%s[DUMATV]%s[ERROR] Парсинг статей DumaTV.ru завершен, но не удалось собрать данные ни с одной из %d страниц.%s\n", ColorBlue, ColorRed, totalLinks, ColorReset)
		if len(errItems) > 0 {
			fmt.Printf("%s[DUMATV]%s[INFO] Список страниц с ошибками или без данных:%s\n", ColorBlue, ColorYellow, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	}

	return products
}
