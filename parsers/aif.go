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
	aifURL        = "https://aif.ru"
	aifURLNews    = "https://aif.ru/news"
	numWorkersAif = 10
)

func AifMain() {
	totalStartTime := time.Now()
	_ = getLinksAif()
	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("%s[AIF]%s[INFO] Парсер Aif.ru заверщил работу: (%s)%s\n", ColorBlue, ColorYellow, FormatDuration(totalElapsedTime), ColorReset)
}

func getLinksAif() []Data {
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

	doc, err := GetHTMLForClient(client, aifURLNews)
	if err != nil {
		fmt.Printf("%s[AIF]%s[ERROR] Ошибка при получении HTML со страницы %s: %v%s\n", ColorBlue, ColorRed, aifURLNews, err, ColorReset)
		return getPageAif(foundLinks)
	}

	doc.Find("div.box_info").Each(func(i int, s *goquery.Selection) {
		aTag := s.Find("a").First()
		href, exists := aTag.Attr("href")
		if exists {
			fullHref := href
			if strings.HasPrefix(href, "/") {
				fullHref = aifURL + href
			}

			if !seenLinks[fullHref] {
				seenLinks[fullHref] = true
				foundLinks = append(foundLinks, fullHref)
			}
		}
	})

	if len(foundLinks) <= 0 {
		fmt.Printf("%s[AIF]%s[WARNING] Не найдено ссылок для парсинга на странице %s.%s\n", ColorBlue, ColorYellow, aifURLNews, ColorReset)
	}

	return getPageAif(foundLinks)
}

type pageParseResultAif struct {
	Data    Data
	Error   error
	PageURL string
	IsEmpty bool
	Reasons []string
}

func getPageAif(links []string) []Data {
	var products []Data
	var errItems []string
	totalLinks := len(links)
	locationPlus3 := time.FixedZone("UTC+3", 3*3600)
	dateTimeStr := "02.01.2006 15:04"

	if totalLinks == 0 {
		return products
	}

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: numWorkersAif + 5,
			IdleConnTimeout:     90 * time.Second,
			MaxConnsPerHost:     numWorkersAif,
		},
	}

	resultsChan := make(chan pageParseResultAif, totalLinks)
	linkChan := make(chan string, totalLinks)

	for _, link := range links {
		linkChan <- link
	}
	close(linkChan)

	var wg sync.WaitGroup

	actualNumWorkers := numWorkersAif
	if totalLinks < numWorkersAif {
		actualNumWorkers = totalLinks
	}

	for i := 0; i < actualNumWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for pageURL := range linkChan {
				var title, body string
				var parsDate time.Time
				var tags []string

				doc, err := GetHTMLForClient(httpClient, pageURL)
				if err != nil {
					resultsChan <- pageParseResultAif{PageURL: pageURL, Error: fmt.Errorf("ошибка GET: %w", err)}
					continue
				}

				title = strings.TrimSpace(doc.Find("h1[itemprop='headline']").First().Text())

				var bodyBuilder strings.Builder
				doc.Find("div.article_text p").Each(func(_ int, s *goquery.Selection) {
					partText := strings.TrimSpace(s.Text())
					if partText != "" {
						if bodyBuilder.Len() > 0 {
							bodyBuilder.WriteString("\n\n")
						}
						bodyBuilder.WriteString(partText)
					}
				})
				body = bodyBuilder.String()

				dateTextRaw := doc.Find("time[itemprop='datePublished']").First().Text()
				dateToParse := strings.TrimSpace(dateTextRaw)

				doc.Find("div.tags span[itemprop='keywords']").Each(func(_ int, s *goquery.Selection) {
					tag := strings.TrimSpace(s.Text())
					if tag != "" {
						tags = append(tags, tag)
					}
				})

				if dateToParse != "" {
					parsedTime, parseErr := time.ParseInLocation(dateTimeStr, dateToParse, locationPlus3)
					if parseErr == nil {
						parsDate = parsedTime
					} else {
						resultsChan <- pageParseResultAif{PageURL: pageURL, Error: fmt.Errorf("ошибка парсинга даты '%s': %w", dateToParse, parseErr)}
						continue
					}
				}

				if title != "" && body != "" && !parsDate.IsZero() {
					dataItem := Data{
						Site:  aifURL,
						Href:  pageURL,
						Title: title,
						Body:  body,
						Date:  parsDate,
						Tags:  tags,
					}
					hash, err := dataItem.Hashing()
					if err != nil {
						resultsChan <- pageParseResultAif{PageURL: pageURL, Error: fmt.Errorf("ошибка генерации хеша: %w", err)}
						continue
					}
					dataItem.Hash = hash
					resultsChan <- pageParseResultAif{Data: dataItem}
				} else {
					var reasons []string
					if title == "" {
						reasons = append(reasons, "T:false")
					}
					if body == "" {
						reasons = append(reasons, "B:false")
					}
					if parsDate.IsZero() {
						reasons = append(reasons, "D:false (исходная строка: '"+dateToParse+"')")
					}
					resultsChan <- pageParseResultAif{PageURL: pageURL, IsEmpty: true, Reasons: reasons}
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
			fmt.Printf("%s[AIF]%s[WARNING] Не удалось обработать %d из %d страниц:%s\n", ColorBlue, ColorYellow, len(errItems), totalLinks, ColorReset)
		}
	} else if totalLinks > 0 {
		fmt.Printf("%s[AIF]%s[ERROR] Парсинг статей Aif.ru завершен, но не удалось собрать данные ни с одной из %d страниц.%s\n", ColorBlue, ColorRed, totalLinks, ColorReset)
		if len(errItems) > 0 {
			fmt.Printf("%s[AIF]%s[INFO] Список страниц с ошибками или без данных:%s\n", ColorBlue, ColorYellow, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	}

	return products
}
