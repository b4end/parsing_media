package parsers

import (
	"fmt"
	. "parsing_media/utils"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	dumatvURL         = "https://dumatv.ru"
	dumatvNewsHTMLURL = "https://dumatv.ru/categories/news"
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
	doc, err := GetHTML(dumatvNewsHTMLURL)
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

func getPageDumaTV(links []string) []Data {
	var products []Data
	var errItems []string
	totalLinks := len(links)

	if totalLinks == 0 {
		return products
	}

	for _, pageURL := range links {
		var title, body string
		var tags []string
		var parsDate time.Time
		parsedSuccessfully := false
		locationPlus3 := time.FixedZone("UTC+3", 3*60*60)
		layout := "2 01 2006 / 15:04"

		doc, err := GetHTML(pageURL)
		if err != nil {
			errItems = append(errItems, fmt.Sprintf("(ошибка GET: %s)", err.Error()))
		} else {
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
				} else if dateToParse != "" {
					// fmt.Printf("%s[DUMATV]%s[DEBUG] Месяц не найден/не заменен в '%s'. Попытка парсинга как '%s'.%s\n", ColorBlue, ColorCyan, dateToParse, processedStr, ColorReset)
				}

				var parseErr error
				parsDate, parseErr = time.ParseInLocation(layout, processedStr, locationPlus3)
				if parseErr != nil {
					fmt.Printf("%s[DUMATV]%s[WARNING] Ошибка парсинга даты: '%s' (попытка с '%s') на %s: %v%s\n", ColorBlue, ColorYellow, dateToParse, processedStr, pageURL, parseErr, ColorReset)
				}
			}

			if title != "" && body != "" && !parsDate.IsZero() && len(tags) != 0 {
				products = append(products, Data{
					Href:  pageURL,
					Title: title,
					Body:  body,
					Date:  parsDate,
					Tags:  tags,
				})
				parsedSuccessfully = true
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
