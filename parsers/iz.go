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
	izURL         = "https://iz.ru"
	izNewsPageURL = "https://iz.ru/news"
	numWorkersIz  = 10
)

func IzMain() {
	totalStartTime := time.Now()
	_ = getLinksIz()
	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("%s[IZ]%s[INFO] Парсер IZ.ru заверщил работу: (%s)%s\n", ColorBlue, ColorYellow, FormatDuration(totalElapsedTime), ColorReset)
}

func getLinksIz() []Data {
	var foundLinks []string
	seenLinks := make(map[string]bool)

	linkSelector1 := "div.view-content div.node__cart__item a.node__cart__item__inside"
	linkSelector2 := "div.short-last-news__inside__list__item a.short-last-news__inside__list__item"
	linkSelector3 := "a.node__cart__item__inside"

	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	doc, err := GetHTMLForClient(client, izNewsPageURL)
	if err != nil {
		fmt.Printf("%s[IZ]%s[ERROR] Ошибка при получении HTML со страницы %s: %v%s\n", ColorBlue, ColorRed, izNewsPageURL, err, ColorReset)
		return getPageIz(foundLinks)
	}

	processLinks := func(s *goquery.Selection, selectorSource string) {
		href, exists := s.Attr("href")
		if exists {
			fullURL := ""
			if strings.HasPrefix(href, "/") {
				fullURL = izURL + href
			} else if strings.HasPrefix(href, izURL) {
				fullURL = href
			}

			if fullURL != "" {
				if idx := strings.Index(fullURL, "?"); idx != -1 {
					fullURL = fullURL[:idx]
				}
				if !seenLinks[fullURL] && strings.Contains(fullURL, izURL+"/") && len(strings.Split(strings.TrimPrefix(fullURL, izURL+"/"), "/")) > 2 {
					seenLinks[fullURL] = true
					foundLinks = append(foundLinks, fullURL)
				}
			}
		}
	}

	doc.Find(linkSelector1).Each(func(i int, s *goquery.Selection) {
		processLinks(s, "1")
	})

	if len(foundLinks) < 5 {
		doc.Find(linkSelector2).Each(func(i int, s *goquery.Selection) {
			processLinks(s, "2")
		})
	}

	if len(foundLinks) < 5 {
		doc.Find(linkSelector3).Each(func(i int, s *goquery.Selection) {
			if s.Closest("div.node__cart__item").Length() > 0 {
				processLinks(s, "3")
			}
		})
	}

	if len(foundLinks) == 0 {
		fmt.Printf("%s[IZ]%s[WARNING] Не найдено ссылок ни с одним из селекторов на странице %s.%s\n", ColorBlue, ColorYellow, izNewsPageURL, ColorReset)
	}

	return getPageIz(foundLinks)
}

type pageParseResultIz struct {
	Data    Data
	Error   error
	PageURL string
	IsEmpty bool
	Reasons []string
}

func getPageIz(links []string) []Data {
	var products []Data
	var errItems []string
	totalLinks := len(links)

	if totalLinks == 0 {
		return products
	}
	tagsAreMandatory := false

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: numWorkersIz + 5,
			IdleConnTimeout:     90 * time.Second,
			MaxConnsPerHost:     numWorkersIz,
		},
	}

	resultsChan := make(chan pageParseResultIz, totalLinks)
	linkChan := make(chan string, totalLinks)

	for _, link := range links {
		linkChan <- link
	}
	close(linkChan)

	var wg sync.WaitGroup

	actualNumWorkers := numWorkersIz
	if totalLinks < numWorkersIz {
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
					resultsChan <- pageParseResultIz{PageURL: pageURL, Error: fmt.Errorf("ошибка GET: %w", err)}
					continue
				}

				title = strings.TrimSpace(doc.Find("h1[itemprop='headline'] span").First().Text())
				if title == "" {
					title = strings.TrimSpace(doc.Find("h1[itemprop='headline']").First().Text())
				}
				if title == "" {
					title = strings.TrimSpace(doc.Find(".article_page__title span").First().Text())
				}
				if title == "" {
					title = strings.TrimSpace(doc.Find(".article_page__title").First().Text())
				}

				var bodyBuilder strings.Builder
				doc.Find("div[itemprop='articleBody'] > div > p").Each(func(j int, s *goquery.Selection) {
					if s.Find("iframe.igi-player").Length() > 0 {
						return
					}
					if s.Parent().Parent().HasClass("more_style_one") {
						return
					}
					if s.Find("a[href*='t.me/izvestia']").Length() > 0 {
						return
					}

					currentTextPart := strings.TrimSpace(s.Text())
					if currentTextPart != "" {
						if bodyBuilder.Len() > 0 {
							bodyBuilder.WriteString("\n\n")
						}
						bodyBuilder.WriteString(currentTextPart)
					}
				})
				body = bodyBuilder.String()
				if body == "" {
					doc.Find("div[itemprop='articleBody'] p").Each(func(j int, s *goquery.Selection) {
						if s.Find("iframe.igi-player").Length() > 0 || s.Parent().Parent().HasClass("more_style_one") || s.Find("a[href*='t.me/izvestia']").Length() > 0 {
							return
						}
						currentTextPart := strings.TrimSpace(s.Text())
						if currentTextPart != "" {
							if bodyBuilder.Len() > 0 {
								bodyBuilder.WriteString("\n\n")
							}
							bodyBuilder.WriteString(currentTextPart)
						}
					})
					body = bodyBuilder.String()
				}

				dateTextRaw := ""
				datetimeAttr, datetimeExists := doc.Find(".article_page__left__top__time__label time").First().Attr("datetime")
				if datetimeExists {
					dateTextRaw = datetimeAttr
				} else {
					datetimeAttr, datetimeExists = doc.Find("time[itemprop='datePublished']").First().Attr("datetime")
					if datetimeExists {
						dateTextRaw = datetimeAttr
					} else {
						dateTextRaw = strings.TrimSpace(doc.Find(".article_page__left__top__time__label time").First().Text())
						if dateTextRaw == "" {
							dateTextRaw = strings.TrimSpace(doc.Find("time[itemprop='datePublished']").First().Text())
						}
					}
				}
				dateToParse := strings.TrimSpace(dateTextRaw)

				if dateToParse != "" {
					parsedTime, parseErr := time.Parse(time.RFC3339, dateToParse)
					if parseErr == nil {
						parsDate = parsedTime
					} else {
						tempDateStr := dateToParse
						for rusM, engMNum := range RussianMonths {
							tempDateStr = strings.ReplaceAll(tempDateStr, rusM, engMNum)
						}

						layoutCustom1 := "2 01 2006, 15:04"
						layoutCustom2 := "2 01 2006 15:04"

						loc := time.FixedZone("MSK", 3*60*60)

						parsedTimeCustom, parseErrCustom := time.ParseInLocation(layoutCustom1, tempDateStr, loc)
						if parseErrCustom != nil {
							parsedTimeCustom, parseErrCustom = time.ParseInLocation(layoutCustom2, tempDateStr, loc)
							if parseErrCustom != nil {
								dateParseError = fmt.Errorf("ошибка парсинга даты '%s' (RFC3339: %v, Custom1: %v, Custom2: %v)", dateToParse, parseErr, parseErrCustom, parseErrCustom)
							} else {
								parsDate = parsedTimeCustom
							}
						} else {
							parsDate = parsedTimeCustom
						}
					}
				} else {
					dateParseError = fmt.Errorf("строка даты пуста")
				}

				if !parsDate.IsZero() && parsDate.Location().String() != "MSK" {
					locationMSK := time.FixedZone("MSK", 3*60*60)
					parsDate = parsDate.In(locationMSK)
				}

				if dateParseError != nil && parsDate.IsZero() {
					fmt.Printf("%s[IZ]%s[WARNING] Ошибка парсинга даты: '%s' на %s: %v%s\n", ColorBlue, ColorYellow, dateToParse, pageURL, dateParseError, ColorReset)
				}

				doc.Find(".hash_tags div[itemprop='about'] a, .article_page__left__tags a").Each(func(_ int, s *goquery.Selection) {
					tagText := strings.TrimSpace(s.Text())
					if tagText != "" {
						tags = append(tags, tagText)
					}
				})

				if title != "" && body != "" && !parsDate.IsZero() && (!tagsAreMandatory || len(tags) > 0) {
					dataItem := Data{
						Site:  izURL,
						Href:  pageURL,
						Title: title,
						Body:  body,
						Date:  parsDate,
						Tags:  tags,
					}
					hash, err := dataItem.Hashing()
					if err != nil {
						resultsChan <- pageParseResultIz{PageURL: pageURL, Error: fmt.Errorf("ошибка генерации хеша: %w", err)}
						continue
					}
					dataItem.Hash = hash
					resultsChan <- pageParseResultIz{Data: dataItem}
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
					resultsChan <- pageParseResultIz{PageURL: pageURL, IsEmpty: true, Reasons: reasons}
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
			fmt.Printf("%s[IZ]%s[WARNING] Не удалось обработать %d из %d страниц (или отсутствовали данные):%s\n", ColorBlue, ColorYellow, len(errItems), totalLinks, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	} else if totalLinks > 0 {
		fmt.Printf("%s[IZ]%s[ERROR] Парсинг статей IZ.ru завершен, но не удалось собрать данные ни с одной из %d страниц.%s\n", ColorBlue, ColorRed, totalLinks, ColorReset)
		if len(errItems) > 0 {
			fmt.Printf("%s[IZ]%s[INFO] Список страниц с ошибками или без данных:%s\n", ColorBlue, ColorYellow, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	}
	return products
}
