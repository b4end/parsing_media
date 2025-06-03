package parsers

import (
	"fmt"
	. "parsing_media/utils"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	vestiURL     = "https://www.vesti.ru"
	vestiURLNews = "https://www.vesti.ru/news"
)

func VestiMain() {
	totalStartTime := time.Now()

	_ = getLinksVesti()

	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("%s[VESTI]%s[INFO] Парсер Vesti.ru заверщил работу: (%s)%s\n", ColorBlue, ColorYellow, FormatDuration(totalElapsedTime), ColorReset)
}

func getLinksVesti() []Data {
	var foundLinks []string
	seenLinks := make(map[string]bool)
	linkSelector := "a.list__pic-wrapper"

	doc, err := GetHTML(vestiURLNews)
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

func getPageVesti(links []string) []Data {
	var products []Data
	var errItems []string
	totalLinks := len(links)

	if totalLinks == 0 {
		return products
	}

	locationPlus3 := time.FixedZone("UTC+3", 3*60*60)
	dateLayoutFromAttr := "2006-01-02 15:04:05"

	dateLayoutFromText := "02 01 2006 15:04"

	for _, pageURL := range links {
		var title, body string
		var tags []string
		var parsDate time.Time
		parsedSuccessfully := false

		doc, err := GetHTML(pageURL)
		if err != nil {
			errItems = append(errItems, fmt.Sprintf("(ошибка GET: %s)", err.Error()))
		} else {
			title = strings.TrimSpace(doc.Find("h1.article__title").First().Text())

			var bodyBuilder strings.Builder
			doc.Find("div.js-mediator-article").Find("p, blockquote").Each(func(_ int, s *goquery.Selection) {
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
			if existsAttr && dateStringAttr != "" {
				var parseErr error
				parsDate, parseErr = time.ParseInLocation(dateLayoutFromAttr, dateStringAttr, locationPlus3)
				if parseErr != nil {
					fmt.Printf("%s[VESTI]%s[WARNING] Ошибка парсинга даты из data-datepub: '%s' (формат '%s') на %s: %v%s\n", ColorBlue, ColorYellow, dateStringAttr, dateLayoutFromAttr, pageURL, parseErr, ColorReset)
				}
			}

			if parsDate.IsZero() {
				dateTextPart := strings.TrimSpace(doc.Find("div.article__date").Contents().Not("span").First().Text())
				timeTextPart := strings.TrimSpace(doc.Find("div.article__date span.article__time").First().Text())

				if dateTextPart != "" && timeTextPart != "" {
					fullDateText := dateTextPart + " " + timeTextPart
					processedDateStr := fullDateText
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
						processedDateStr = tempProcessedStr
					}

					var parseErr error
					parsDate, parseErr = time.ParseInLocation(dateLayoutFromText, processedDateStr, locationPlus3)
					if parseErr != nil {
						fmt.Printf("%s[VESTI]%s[WARNING] Ошибка парсинга даты из текста: '%s' (попытка с '%s', формат '%s') на %s: %v%s\n", ColorBlue, ColorYellow, fullDateText, processedDateStr, dateLayoutFromText, pageURL, parseErr, ColorReset)
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

			if title != "" && body != "" && !parsDate.IsZero() && len(tags) > 0 {
				products = append(products, Data{
					Href:  pageURL,
					Title: title,
					Body:  body,
					Date:  parsDate,
					Tags:  tags,
				})
				parsedSuccessfully = true
			}
		}

		if !parsedSuccessfully && err == nil {
			var reasons []string
			if title == "" {
				reasons = append(reasons, "T:false")
			}
			if body == "" {
				reasons = append(reasons, "B:false")
			}
			if parsDate.IsZero() {
				reasons = append(reasons, "D:false")
			}
			if len(tags) == 0 {
				reasons = append(reasons, "Tags:false")
			}
			errItems = append(errItems, fmt.Sprintf("%s (нет данных: %s)", pageURL, strings.Join(reasons, ", ")))
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

	return products
}
