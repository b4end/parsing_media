package parsers

import (
	"fmt"
	. "parsing_media/utils"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	gazetaURL     = "https://www.gazeta.ru"
	gazetaURLNews = "https://www.gazeta.ru/news/"
)

func GazetaMain() {
	totalStartTime := time.Now()

	_ = getLinksGazeta()

	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("%s[GAZETA]%s[INFO] Парсер Gazeta.ru заверщил работу: (%s)%s\n", ColorBlue, ColorYellow, FormatDuration(totalElapsedTime), ColorReset)
}

func getLinksGazeta() []Data {
	var foundLinks []string
	seenLinks := make(map[string]bool)
	doc, err := GetHTML(gazetaURLNews)
	if err != nil {
		fmt.Printf("%s[GAZETA]%s[ERROR] Не удалось загрузить основную страницу новостей %s после всех попыток. Сбор ссылок прерван.%s\n", ColorBlue, ColorRed, gazetaURLNews, ColorReset)
		return getPageGazeta(foundLinks)
	}
	doc.Find("a.b_ear.m_techlisting").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists {
			fullHref := ""
			if strings.HasPrefix(href, "/") {
				fullHref = gazetaURL + href
			} else if strings.HasPrefix(href, gazetaURL) {
				fullHref = href
			}
			if fullHref != "" && !seenLinks[fullHref] {
				seenLinks[fullHref] = true
				foundLinks = append(foundLinks, fullHref)
			}
		}
	})
	if len(foundLinks) == 0 {
		fmt.Printf("%s[GAZETA]%s[WARNING] Не найдено ссылок с селектором 'a.b_ear.m_techlisting' на странице %s.%s\n", ColorBlue, ColorYellow, gazetaURLNews, ColorReset)
	}
	return getPageGazeta(foundLinks)
}

func getPageGazeta(links []string) []Data {
	var products []Data
	var errItems []string
	totalLinks := len(links)

	if totalLinks == 0 {
		return products
	}

	tagsAreMandatoryForThisParser := true

	for _, pageURL := range links {
		var title, body string
		var tags []string
		var parsDate time.Time
		parsedSuccessfully := false

		doc, err := GetHTML(pageURL)
		if err != nil {
			errItems = append(errItems, fmt.Sprintf("(ошибка GET: %s)", err.Error()))
		} else {
			title = strings.TrimSpace(doc.Find("h1.headline").First().Text())

			var accumulatedBodyParts []string
			doc.Find("div.b_article-text p").Each(func(_ int, pSelection *goquery.Selection) {
				paragraphText := strings.TrimSpace(pSelection.Text())
				if paragraphText != "" &&
					!strings.Contains(paragraphText, "Что думаешь?") &&
					!strings.HasPrefix(paragraphText, "Ранее ") {
					accumulatedBodyParts = append(accumulatedBodyParts, paragraphText)
				}
			})
			body = strings.Join(accumulatedBodyParts, "\n\n")

			if title != "" && body != "" && strings.HasPrefix(body, title) {
				body = strings.TrimPrefix(body, title)
				body = strings.TrimSpace(body)
			}

			dateSelector := `time.time[itemprop="datePublished"]`
			dateStr, exists := doc.Find(dateSelector).Attr("datetime")
			if exists {
				parsedTime, err := time.Parse(time.RFC3339, dateStr)
				if err != nil {
					fmt.Printf("%s[GAZETA]%s[WARNING] Ошибка парсинга даты из атрибута 'datetime': '%s' (селектор: '%s') на %s: %v%s\n", ColorBlue, ColorYellow, dateStr, dateSelector, pageURL, err, ColorReset)
				} else {
					parsDate = parsedTime
				}
			} else {
				fmt.Printf("%s[GAZETA]%s[INFO] Атрибут 'datetime' с датой не найден (селектор: '%s') на %s%s\n", ColorBlue, ColorYellow, dateSelector, pageURL, ColorReset)
			}

			rubricSelector := `div.b_article-breadcrumb-item a.rubric`
			rubricText := strings.TrimSpace(doc.Find(rubricSelector).First().Text())
			if rubricText != "" {
				tags = append(tags, rubricText)
			} else {
				if tagsAreMandatoryForThisParser {
					// fmt.Printf("%s[GAZETA]%s[INFO] Обязательная рубрика (тег) не найдена (селектор: '%s') на %s%s\n", ColorBlue, ColorCyan, rubricSelector, pageURL, ColorReset)
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
			fmt.Printf("%s[GAZETA]%s[WARNING] Не удалось обработать %d из %d страниц (или отсутствовали данные):%s\n", ColorBlue, ColorYellow, len(errItems), totalLinks, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	} else if totalLinks > 0 {
		fmt.Printf("%s[GAZETA]%s[ERROR] Парсинг статей Gazeta.ru завершен, но не удалось собрать данные ни с одной из %d страниц.%s\n", ColorBlue, ColorRed, totalLinks, ColorReset)
		if len(errItems) > 0 {
			fmt.Printf("%s[GAZETA]%s[INFO] Список страниц с ошибками или без данных:%s\n", ColorBlue, ColorYellow, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	}
	return products
}
