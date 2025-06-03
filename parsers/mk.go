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
	mkURL         = "https://www.mk.ru"
	mkNewsPageURL = "https://www.mk.ru/news/"
	numWorkersMK  = 10
	mkDateLayout  = "2006-01-02T15:04:05-0700"
)

func MKMain() {
	totalStartTime := time.Now()
	_ = getLinksMK()
	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("%s[MK]%s[INFO] Парсер MK.ru заверщил работу: (%s)%s\n", ColorBlue, ColorYellow, FormatDuration(totalElapsedTime), ColorReset)
}

func getLinksMK() []Data {
	var foundLinks []string
	seenLinks := make(map[string]bool)

	targetURL := mkNewsPageURL
	linkSelector := "a.news-listing__item-link"

	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	doc, err := GetHTMLForClient(client, targetURL)
	if err != nil {
		fmt.Printf("%s[MK]%s[ERROR] Ошибка при получении HTML со страницы %s: %v%s\n", ColorBlue, ColorRed, targetURL, err, ColorReset)
		return getPageMK(foundLinks)
	}

	doc.Find(linkSelector).Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists {
			isAd := s.Find("h3.news-listing__item_ad").Length() > 0
			if !isAd && strings.HasPrefix(href, mkURL) {
				if !seenLinks[href] {
					seenLinks[href] = true
					foundLinks = append(foundLinks, href)
				}
			}
		}
	})

	if len(foundLinks) == 0 {
		fmt.Printf("%s[MK]%s[WARNING] Не найдено ссылок с селектором '%s' на странице %s (или все найденные ссылки являются рекламными).%s\n", ColorBlue, ColorYellow, linkSelector, targetURL, ColorReset)
	}

	limit := 50
	if len(foundLinks) < limit {
		limit = len(foundLinks)
	}
	return getPageMK(foundLinks[:limit])
}

type pageParseResultMK struct {
	Data    Data
	Error   error
	PageURL string
	IsEmpty bool
	Reasons []string
}

func getPageMK(links []string) []Data {
	var products []Data
	var errItems []string
	totalLinks := len(links)

	if totalLinks == 0 {
		return products
	}

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: numWorkersMK + 5,
			IdleConnTimeout:     90 * time.Second,
			MaxConnsPerHost:     numWorkersMK,
		},
	}

	resultsChan := make(chan pageParseResultMK, totalLinks)
	linkChan := make(chan string, totalLinks)

	for _, link := range links {
		linkChan <- link
	}
	close(linkChan)

	var wg sync.WaitGroup

	actualNumWorkers := numWorkersMK
	if totalLinks < numWorkersMK {
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
					resultsChan <- pageParseResultMK{PageURL: pageURL, Error: fmt.Errorf("ошибка GET: %w", err)}
					continue
				}

				title = strings.TrimSpace(doc.Find("h1.article__title").First().Text())

				var bodyBuilder strings.Builder
				doc.Find("div.article__body p").Each(func(_ int, s *goquery.Selection) {
					partText := strings.TrimSpace(s.Text())
					if partText != "" {
						if strings.Contains(partText, "Самые яркие фото и видео дня") && strings.Contains(partText, "Telegram-канале") {
							return
						}
						if (strings.HasPrefix(partText, "Читайте также:") || strings.HasPrefix(partText, "Смотрите видео по теме:")) && s.Find("a").Length() > 0 {
							return
						}
						if bodyBuilder.Len() > 0 {
							bodyBuilder.WriteString("\n\n")
						}
						bodyBuilder.WriteString(partText)
					}
				})
				body = bodyBuilder.String()

				dateString, exists := doc.Find("time.meta__text[datetime]").Attr("datetime")
				if exists && dateString != "" {
					var parseErr error
					parsDate, parseErr = time.Parse(mkDateLayout, dateString)
					if parseErr != nil {
						fmt.Printf("%s[MK]%s[WARNING] Ошибка парсинга даты: '%s' (формат '%s') на %s: %v%s\n", ColorBlue, ColorYellow, dateString, mkDateLayout, pageURL, parseErr, ColorReset)
						// Continue processing even if date parsing fails, but date will be zero
					}
				}

				if title != "" && body != "" && !parsDate.IsZero() {
					resultsChan <- pageParseResultMK{Data: Data{
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
						reasons = append(reasons, "D:false (исходная строка: '"+dateString+"')")
					}
					resultsChan <- pageParseResultMK{PageURL: pageURL, IsEmpty: true, Reasons: reasons}
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
			fmt.Printf("%s[MK]%s[WARNING] Не удалось полностью обработать %d из %d страниц (или отсутствовали некоторые данные):%s\n", ColorBlue, ColorYellow, len(errItems), totalLinks, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	} else if totalLinks > 0 {
		fmt.Printf("%s[MK]%s[ERROR] Парсинг статей MK.ru завершен, но не удалось собрать данные ни с одной из %d страниц.%s\n", ColorBlue, ColorRed, totalLinks, ColorReset)
		if len(errItems) > 0 {
			fmt.Printf("%s[MK]%s[INFO] Список страниц с ошибками или без данных:%s\n", ColorBlue, ColorYellow, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	}

	return products
}
