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
	vestiURL        = "https://www.vesti.ru"
	vestiURLNews    = "https://www.vesti.ru/news"
	numWorkersVesti = 10
)

func VestiMain() {
	totalStartTime := time.Now()
	articles, links := getLinksVesti()
	SaveData(articles)
	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("%s[VESTI]%s[INFO] Парсер Vesti.ru заверщил работу собрав (%d/%d): (%s)%s\n", ColorBlue, ColorYellow, len(articles), len(links), FormatDuration(totalElapsedTime), ColorReset)
}

func getLinksVesti() ([]Data, []string) {
	var foundLinks []string
	seenLinks := make(map[string]bool)
	linkSelector := "a.list__pic-wrapper"

	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	doc, err := GetHTMLForClient(client, vestiURLNews)
	if err != nil {
		fmt.Printf("%s[VESTI]%s[ERROR] Ошибка при получении HTML со страницы %s: %v%s\n", ColorBlue, ColorRed, vestiURLNews, err, ColorReset)
		return getPageVesti(foundLinks)
	}

	doc.Find(linkSelector).Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists {
			fullHref := ""
			if strings.HasPrefix(href, "/") {
				fullHref = vestiURL + href
			} else if strings.HasPrefix(href, vestiURL) {
				fullHref = href
			}

			if fullHref != "" && !seenLinks[fullHref] {
				seenLinks[fullHref] = true
				foundLinks = append(foundLinks, fullHref)
			}
		}
	})

	if len(foundLinks) == 0 {
		fmt.Printf("%s[VESTI]%s[WARNING] Не найдено ссылок с селектором '%s' на странице %s.%s\n", ColorBlue, ColorYellow, linkSelector, vestiURLNews, ColorReset)
	}

	return getPageVesti(foundLinks)
}

type pageParseResultVesti struct {
	Data    Data
	Error   error
	PageURL string
	IsEmpty bool
	Reasons []string
}

func getPageVesti(links []string) ([]Data, []string) {
	var products []Data
	var errItems []string
	totalLinks := len(links)

	if totalLinks == 0 {
		return products, links
	}

	locationPlus3 := time.FixedZone("UTC+3", 3*60*60)
	dateLayoutFromAttr := "2006-01-02 15:04:05"
	dateLayoutFromText := "02 01 2006 15:04"
	tagsAreMandatory := true

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: numWorkersVesti + 5,
			IdleConnTimeout:     90 * time.Second,
			MaxConnsPerHost:     numWorkersVesti,
		},
	}

	resultsChan := make(chan pageParseResultVesti, totalLinks)
	linkChan := make(chan string, totalLinks)

	for _, link := range links {
		linkChan <- link
	}
	close(linkChan)

	var wg sync.WaitGroup

	actualNumWorkers := numWorkersVesti
	if totalLinks < numWorkersVesti {
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
				var dateParseErrorAttr, dateParseErrorText error
				var originalDateStrAttr, originalDateStrText, processedDateStrText string

				doc, err := GetHTMLForClient(httpClient, pageURL)
				if err != nil {
					resultsChan <- pageParseResultVesti{PageURL: pageURL, Error: fmt.Errorf("ошибка GET: %w", err)}
					continue
				}

				title = strings.TrimSpace(doc.Find("h1.article__title").First().Text())

				var bodyBuilder strings.Builder
				doc.Find("div.js-mediator-article > p, div.js-mediator-article > blockquote").Each(func(_ int, s *goquery.Selection) {
					partText := strings.TrimSpace(s.Text())
					if partText != "" {
						if bodyBuilder.Len() > 0 {
							bodyBuilder.WriteString("\n\n")
						}
						bodyBuilder.WriteString(partText)
					}
				})
				body = bodyBuilder.String()

				dateStringAttr, existsAttr := doc.Find("article.article[data-datepub]").Attr("data-datepub")
				originalDateStrAttr = dateStringAttr
				if existsAttr && dateStringAttr != "" {
					parsedTime, parseErr := time.ParseInLocation(dateLayoutFromAttr, dateStringAttr, locationPlus3)
					if parseErr != nil {
						dateParseErrorAttr = parseErr
						fmt.Printf("%s[VESTI]%s[WARNING] Ошибка парсинга даты из data-datepub: '%s' (формат '%s') на %s: %v%s\n", ColorBlue, ColorYellow, dateStringAttr, dateLayoutFromAttr, pageURL, parseErr, ColorReset)
					} else {
						parsDate = parsedTime
					}
				}

				if parsDate.IsZero() {
					dateTextPart := strings.TrimSpace(doc.Find("div.article__date").Contents().Not("span").First().Text())
					timeTextPart := strings.TrimSpace(doc.Find("div.article__date span.article__time").First().Text())

					if dateTextPart != "" && timeTextPart != "" {
						fullDateText := dateTextPart + " " + timeTextPart
						originalDateStrText = fullDateText
						processedStr := fullDateText
						foundMonth := false
						lowerDateToParse := strings.ToLower(fullDateText)
						tempProcessedStr := fullDateText

						for rusMonth, numMonth := range RussianMonths {
							lowerRusMonth := strings.ToLower(rusMonth)
							if strings.Contains(lowerDateToParse, lowerRusMonth) {
								startIndex := strings.Index(lowerDateToParse, lowerRusMonth)
								if startIndex != -1 {
									tempProcessedStr = fullDateText[:startIndex] + numMonth + fullDateText[startIndex+len(rusMonth):]
									foundMonth = true
									break
								}
							}
						}
						if foundMonth {
							processedStr = tempProcessedStr
						}
						processedDateStrText = processedStr

						parsedTime, parseErr := time.ParseInLocation(dateLayoutFromText, processedStr, locationPlus3)
						if parseErr != nil {
							dateParseErrorText = parseErr
							fmt.Printf("%s[VESTI]%s[WARNING] Ошибка парсинга даты из текста: '%s' (попытка с '%s', формат '%s') на %s: %v%s\n", ColorBlue, ColorYellow, fullDateText, processedStr, dateLayoutFromText, pageURL, parseErr, ColorReset)
						} else {
							parsDate = parsedTime
						}
					}
				}

				categoryTag := strings.TrimSpace(doc.Find("div.article__date div.list__subtitle a.list__src").First().Text())
				if categoryTag != "" {
					tags = append(tags, categoryTag)
				}

				doc.Find("div.tags a.tags__item").Each(func(_ int, s *goquery.Selection) {
					tagText := strings.TrimSpace(s.Text())
					if tagText != "" {
						isDuplicate := false
						for _, existingTag := range tags {
							if existingTag == tagText {
								isDuplicate = true
								break
							}
						}
						if !isDuplicate {
							tags = append(tags, tagText)
						}
					}
				})

				if title != "" && body != "" && !parsDate.IsZero() && (!tagsAreMandatory || len(tags) > 0) {
					dataItem := Data{
						Site:  vestiURL,
						Href:  pageURL,
						Title: title,
						Body:  body,
						Date:  parsDate,
						Tags:  tags,
					}
					hash, err := dataItem.Hashing()
					if err != nil {
						resultsChan <- pageParseResultVesti{PageURL: pageURL, Error: fmt.Errorf("ошибка генерации хеша: %w", err)}
						continue
					}
					dataItem.Hash = hash
					resultsChan <- pageParseResultVesti{Data: dataItem}
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
						if dateParseErrorAttr != nil && dateParseErrorText != nil {
							reasonDate = fmt.Sprintf("D:false (attr_err: %v, attr_str: '%s'; text_err: %v, text_str_orig: '%s', text_str_proc: '%s')", dateParseErrorAttr, originalDateStrAttr, dateParseErrorText, originalDateStrText, processedDateStrText)
						} else if dateParseErrorAttr != nil {
							reasonDate = fmt.Sprintf("D:false (attr_err: %v, attr_str: '%s')", dateParseErrorAttr, originalDateStrAttr)
						} else if dateParseErrorText != nil {
							reasonDate = fmt.Sprintf("D:false (text_err: %v, text_str_orig: '%s', text_str_proc: '%s')", dateParseErrorText, originalDateStrText, processedDateStrText)
						} else if !existsAttr && originalDateStrText == "" {
							reasonDate = "D:false (no_source)"
						}
						reasons = append(reasons, reasonDate)
					}
					if tagsAreMandatory && len(tags) == 0 {
						reasons = append(reasons, "Tags:false")
					}
					resultsChan <- pageParseResultVesti{PageURL: pageURL, IsEmpty: true, Reasons: reasons}
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
			fmt.Printf("%s[VESTI]%s[WARNING] Не удалось обработать %d из %d страниц (или отсутствовали данные):%s\n", ColorBlue, ColorYellow, len(errItems), totalLinks, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	} else if totalLinks > 0 {
		fmt.Printf("%s[VESTI]%s[ERROR] Парсинг статей Vesti.ru завершен, но не удалось собрать данные ни с одной из %d страниц.%s\n", ColorBlue, ColorRed, totalLinks, ColorReset)
		if len(errItems) > 0 {
			fmt.Printf("%s[VESTI]%s[INFO] Список страниц с ошибками или без данных:%s\n", ColorBlue, ColorYellow, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	}

	return products, links
}
