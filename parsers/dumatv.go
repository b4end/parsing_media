package parsers

import (
	"fmt"
	. "parsing_media/utils" // Assuming Data, ColorBlue, ColorYellow, ColorRed, ColorReset, GetHTML, FormatDuration, LimitString are defined here
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

	_ = getLinksDumaTV() // Assuming we don't need the result for now, like in BankiMain

	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("\n%s[DUMATV]%s[INFO] Общее время выполнения парсера DumaTV.ru: %s%s\n", ColorBlue, ColorYellow, FormatDuration(totalElapsedTime), ColorReset)
}

func getLinksDumaTV() []Data {
	var foundLinks []string
	seenLinks := make(map[string]bool)

	doc, err := GetHTML(dumatvNewsHTMLURL)
	if err != nil {
		fmt.Printf("%s[DUMATV]%s[ERROR] Ошибка при получении HTML со страницы %s: %v%s\n", ColorBlue, ColorRed, dumatvNewsHTMLURL, err, ColorReset)
		return getPageDumaTV(foundLinks) // Proceed with empty links to allow getPageDumaTV to print its "no links" message if needed
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
		// This message is fine as is, getLinksDumaTV already warns if no links were found from the source.
		// If getLinksDumaTV itself failed and returned empty links, this acts as a secondary indicator.
		// fmt.Printf("%s[DUMATV]%s[INFO] Нет ссылок для парсинга статей.%s\n", ColorBlue, ColorYellow, ColorReset)
		return products
	}

	for _, pageURL := range links {
		var title, body, pageDate string
		parsedSuccessfully := false

		doc, err := GetHTML(pageURL)
		if err != nil {
			errItems = append(errItems, fmt.Sprintf("%s (ошибка GET: %s)", LimitString(pageURL, 60), LimitString(err.Error(), 50)))
		} else {
			title = strings.TrimSpace(doc.Find("h1.news-post-content__title").First().Text())

			rawDateTimeStr := strings.TrimSpace(doc.Find("div.news-post-top__date").First().Text())
			parts := strings.Split(rawDateTimeStr, " / ")
			if len(parts) == 2 {
				pageDate = fmt.Sprint(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
			} else if rawDateTimeStr != "" {
				// This is an in-loop warning specific to data quality. Keep its format consistent.
				fmt.Printf("%s[DUMATV]%s[WARNING] Неожиданный формат даты/времени: '%s' на %s%s\n", ColorBlue, ColorYellow, rawDateTimeStr, pageURL, ColorReset)
			}

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

			if title != "" && body != "" && pageDate != "" { // Assuming Date is crucial for DumaTV
				products = append(products, Data{
					Title: title,
					Body:  body,
					Href:  pageURL,
					Date:  pageDate,
				})
				parsedSuccessfully = true
			}
		}

		if !parsedSuccessfully && err == nil { // Only add to errItems if GetHTML was successful but data extraction failed
			errItems = append(errItems, fmt.Sprintf("%s (нет данных: T:%t, B:%t, D:%t)", LimitString(pageURL, 60), title != "", body != "", pageDate != ""))
		}
	}

	if len(products) > 0 {
		if len(errItems) > 0 {
			fmt.Printf("%s[DUMATV]%s[WARNING] Не удалось обработать %d из %d страниц:%s\n", ColorBlue, ColorYellow, len(errItems), totalLinks, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	} else if totalLinks > 0 { // No products collected, but links were attempted
		fmt.Printf("\n%s[DUMATV]%s[ERROR] Парсинг статей DumaTV.ru завершен, но не удалось собрать данные ни с одной из %d страниц.%s\n", ColorBlue, ColorRed, totalLinks, ColorReset)
		if len(errItems) > 0 {
			fmt.Printf("%s[DUMATV]%s[INFO] Список страниц с ошибками или без данных:%s\n", ColorBlue, ColorYellow, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	}
	// If totalLinks == 0 initially, no summary message about processing is needed,
	// as getLinksDumaTV would have already logged a warning or error.

	return products
}
