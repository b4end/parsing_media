package parsers

import (
	"fmt"
	. "parsing_media/utils"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	riaURL         = "https://ria.ru"
	riaNewsPageURL = "https://ria.ru/lenta/"
)

func RiaMain() {
	totalStartTime := time.Now()

	_ = getLinksRia()

	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("%s[RIA]%s[INFO] Парсер RIA.ru заверщил работу: (%s)%s\n", ColorBlue, ColorYellow, FormatDuration(totalElapsedTime), ColorReset)
}

func getLinksRia() []Data {
	var foundLinks []string
	seenLinks := make(map[string]bool)
	linkSelector := "a.list-item__title.color-font-hover-only"

	doc, err := GetHTML(riaNewsPageURL)
	if err != nil {
		fmt.Printf("%s[RIA]%s[ERROR] Ошибка при получении HTML со страницы %s: %v%s\n", ColorBlue, ColorRed, riaNewsPageURL, err, ColorReset)
		return getPageRia(foundLinks)
	}

	doc.Find(linkSelector).Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists {
			if strings.HasPrefix(href, "https://ria.ru") {
				if !seenLinks[href] {
					seenLinks[href] = true
					foundLinks = append(foundLinks, href)
				}
			}
		}
	})

	if len(foundLinks) == 0 {
		fmt.Printf("%s[RIA]%s[WARNING] Не найдено ссылок с селектором '%s' на странице %s.%s\n", ColorBlue, ColorYellow, linkSelector, riaNewsPageURL, ColorReset)
	}

	return getPageRia(foundLinks)
}

func getPageRia(links []string) []Data {
	var products []Data
	var errItems []string
	totalLinks := len(links)

	if totalLinks == 0 {
		return products
	}

	locationPlus3 := time.FixedZone("UTC+3", 3*60*60)
	dateLayout := "15:04 02.01.2006"

	for _, pageURL := range links {
		var title, body string
		var tags []string
		var parsDate time.Time
		parsedSuccessfully := false

		doc, err := GetHTML(pageURL)
		if err != nil {
			errItems = append(errItems, fmt.Sprintf("(ошибка GET: %s)", err.Error()))
		} else {
			title = strings.TrimSpace(doc.Find(".article__title").First().Text())

			var bodyBuilder strings.Builder
			var targetNodes *goquery.Selection
			articleBodyNode := doc.Find(".article__body")
			if articleBodyNode.Length() > 0 {
				targetNodes = articleBodyNode.Find(".article__text, .article__quote-text")
			} else {
				targetNodes = doc.Find(".article__text, .article__quote-text")
			}

			targetNodes.Each(func(j int, s *goquery.Selection) {
				currentTextPart := strings.TrimSpace(s.Text())
				if currentTextPart != "" {
					if bodyBuilder.Len() > 0 {
						bodyBuilder.WriteString("\n\n")
					}
					bodyBuilder.WriteString(currentTextPart)
				}
			})
			body = bodyBuilder.String()

			dateTextRaw := doc.Find("div.article__info-date > a").First().Text()
			dateToParse := strings.TrimSpace(dateTextRaw)

			if dateToParse != "" {
				var parseErr error
				parsDate, parseErr = time.ParseInLocation(dateLayout, dateToParse, locationPlus3)
				if parseErr != nil {
					fmt.Printf("%s[RIA]%s[WARNING] Ошибка парсинга даты: '%s' (формат '%s') на %s: %v%s\n", ColorBlue, ColorYellow, dateToParse, dateLayout, pageURL, parseErr, ColorReset)
				}
			} else {
				// fmt.Printf("%s[RIA]%s[DEBUG] Текст даты не найден на %s%s\n", ColorBlue, ColorCyan, pageURL, ColorReset)
			}

			doc.Find("div.article__tags a.article__tags-item").Each(func(_ int, s *goquery.Selection) {
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
			fmt.Printf("%s[RIA]%s[WARNING] Не удалось обработать %d из %d страниц (или отсутствовали данные):%s\n", ColorBlue, ColorYellow, len(errItems), totalLinks, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	} else if totalLinks > 0 {
		fmt.Printf("%s[RIA]%s[ERROR] Парсинг статей RIA.ru завершен, но не удалось собрать данные ни с одной из %d страниц.%s\n", ColorBlue, ColorRed, totalLinks, ColorReset)
		if len(errItems) > 0 {
			fmt.Printf("%s[RIA]%s[INFO] Список страниц с ошибками или без данных:%s\n", ColorBlue, ColorYellow, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	}
	return products
}
