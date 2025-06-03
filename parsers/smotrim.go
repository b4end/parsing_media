package parsers

import (
	"fmt"
	. "parsing_media/utils"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	smotrimURL         = "https://smotrim.ru"
	smotrimNewsHTMLURL = "https://smotrim.ru/articles"
)

func SmotrimMain() {
	totalStartTime := time.Now()

	_ = getLinksSmotrim()

	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("%s[SMOTRIM]%s[INFO] Парсер Smotrim.ru заверщил работу: (%s)%s\n", ColorBlue, ColorYellow, FormatDuration(totalElapsedTime), ColorReset)
}

func getLinksSmotrim() []Data {
	var foundLinks []string
	seenLinks := make(map[string]bool)

	doc, err := GetHTML(smotrimNewsHTMLURL)
	if err != nil {
		fmt.Printf("%s[SMOTRIM]%s[ERROR] Ошибка при получении HTML со страницы %s: %v%s\n", ColorBlue, ColorRed, smotrimNewsHTMLURL, err, ColorReset)
		return getPageSmotrim(foundLinks)
	}

	linkSelector := "li.list-item--article h3.list-item__title a.list-item__link"

	doc.Find(linkSelector).Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists {
			fullHref := ""
			if strings.HasPrefix(href, "/") {
				fullHref = smotrimURL + href
			} else if strings.HasPrefix(href, smotrimURL) {
				fullHref = href
			}

			if fullHref != "" && !seenLinks[fullHref] {
				seenLinks[fullHref] = true
				foundLinks = append(foundLinks, fullHref)
			}
		}
	})

	if len(foundLinks) == 0 {
		fmt.Printf("%s[SMOTRIM]%s[WARNING] Не найдено ссылок с селектором '%s' на странице %s.%s\n", ColorBlue, ColorYellow, linkSelector, smotrimNewsHTMLURL, ColorReset)
	}

	return getPageSmotrim(foundLinks)
}

func getPageSmotrim(links []string) []Data {
	var products []Data
	var errItems []string
	totalLinks := len(links)

	if totalLinks == 0 {
		return products
	}

	locationPlus3 := time.FixedZone("UTC+3", 3*60*60)
	dateLayout := "2 01 2006, 15:04"

	for _, pageURL := range links {
		var title, body string
		var tags []string
		var parsDate time.Time
		parsedSuccessfully := false

		doc, err := GetHTML(pageURL)
		if err != nil {
			errItems = append(errItems, fmt.Sprintf("(ошибка GET: %s)", err.Error()))
		} else {
			title = strings.TrimSpace(doc.Find("h1.article-main-item__title").First().Text())

			var bodyBuilder strings.Builder
			doc.Find("div.article-main-item__body").Find("p, blockquote").Each(func(_ int, s *goquery.Selection) {
				partText := strings.TrimSpace(s.Text())
				if partText != "" {
					if strings.Contains(partText, "Все видео материалы по теме:") ||
						strings.Contains(partText, "Материалы по теме") ||
						strings.HasPrefix(partText, "Смотрите также:") ||
						strings.HasPrefix(partText, "Читайте также:") {
						return
					}

					if bodyBuilder.Len() > 0 {
						bodyBuilder.WriteString("\n\n")
					}
					bodyBuilder.WriteString(partText)
				}
			})
			body = bodyBuilder.String()

			dateTextRaw := doc.Find("span.r_offset_0").First().Text()
			if dateTextRaw == "" {
				dateTextRaw = doc.Find("div.article-main-item__date").First().Text()
			}
			if dateTextRaw == "" {
				dateTextRaw = doc.Find(".article__date").First().Text()
			}

			dateToParse := strings.TrimSpace(dateTextRaw)
			processedDateStr := dateToParse

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

				var parseErr error
				parsDate, parseErr = time.ParseInLocation(dateLayout, processedDateStr, locationPlus3)
				if parseErr != nil {
					fmt.Printf("%s[SMOTRIM]%s[WARNING] Ошибка парсинга даты: '%s' (попытка с '%s', формат '%s') на %s: %v%s\n", ColorBlue, ColorYellow, dateToParse, processedDateStr, dateLayout, pageURL, parseErr, ColorReset)
				}
			}

			doc.Find("div.tags-list__content ul.tags-list__list li.tags-list__item a.tags-list__link").Each(func(_ int, s *goquery.Selection) {
				tagText := strings.TrimSpace(s.Text())
				if tagText != "" {
					tags = append(tags, tagText)
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
			fmt.Printf("%s[SMOTRIM]%s[WARNING] Не удалось обработать %d из %d страниц (или отсутствовали данные):%s\n", ColorBlue, ColorYellow, len(errItems), totalLinks, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	} else if totalLinks > 0 {
		fmt.Printf("%s[SMOTRIM]%s[ERROR] Парсинг статей Smotrim.ru завершен, но не удалось собрать данные ни с одной из %d страниц.%s\n", ColorBlue, ColorRed, totalLinks, ColorReset)
		if len(errItems) > 0 {
			fmt.Printf("%s[SMOTRIM]%s[INFO] Список страниц с ошибками или без данных:%s\n", ColorBlue, ColorYellow, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	}

	return products
}
