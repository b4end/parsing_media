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

	fmt.Printf("%s[INFO] Запуск парсера DumaTV.ru (HTML)...%s\n", ColorYellow, ColorReset)
	_ = getLinksDumaTV()

	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("\n%s[INFO] Общее время выполнения парсера DumaTV.ru: %s%s\n", ColorYellow, FormatDuration(totalElapsedTime), ColorReset)
}

func getLinksDumaTV() []Data {
	var foundLinks []string
	seenLinks := make(map[string]bool)

	fmt.Printf("%s[INFO] Начало парсинга ссылок с HTML-страницы %s...%s\n", ColorYellow, dumatvNewsHTMLURL, ColorReset)

	doc, err := GetHTML(dumatvNewsHTMLURL)
	if err != nil {
		fmt.Printf("%s[ERROR] Ошибка при получении HTML со страницы %s: %v%s\n", ColorRed, dumatvNewsHTMLURL, err, ColorReset)
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

	if len(foundLinks) > 0 {
		fmt.Printf("%s[INFO] Найдено %d уникальных ссылок на статьи с HTML-страницы.%s\n", ColorGreen, len(foundLinks), ColorReset)
	} else {
		fmt.Printf("%s[WARNING] Не найдено ссылок с селектором '%s' на странице %s.%s\n", ColorYellow, linkSelector, dumatvNewsHTMLURL, ColorReset)
	}

	return getPageDumaTV(foundLinks)
}

func getPageDumaTV(links []string) []Data {
	var products []Data
	var errItems []string
	totalLinks := len(links)

	if totalLinks == 0 {
		fmt.Printf("%s[INFO] Нет ссылок для парсинга статей.%s\n", ColorYellow, ColorReset)
		return products
	}
	fmt.Printf("\n%s[INFO] Начало парсинга %d статей с DumaTV.ru...%s\n", ColorYellow, totalLinks, ColorReset)

	for i, pageURL := range links {
		var title, body, pageDate, pageTime string
		var pageStatusMessage string
		var statusMessageColor = ColorReset
		parsedSuccessfully := false

		doc, err := GetHTML(pageURL)
		if err != nil {
			pageStatusMessage = fmt.Sprintf("Ошибка GET: %s", LimitString(err.Error(), 50))
			statusMessageColor = ColorRed
			errItems = append(errItems, fmt.Sprintf("%s (ошибка GET: %s)", LimitString(pageURL, 60), LimitString(err.Error(), 50)))
		} else {
			title = strings.TrimSpace(doc.Find("h1.news-post-content__title").First().Text())

			rawDateTimeStr := strings.TrimSpace(doc.Find("div.news-post-top__date").First().Text())
			parts := strings.Split(rawDateTimeStr, " / ")
			if len(parts) == 2 {
				pageDate = strings.TrimSpace(parts[0])
				pageTime = strings.TrimSpace(parts[1])
			} else if rawDateTimeStr != "" {
				fmt.Printf("\n%s[WARNING] Неожиданный формат даты/времени: '%s' на %s%s\n", ColorYellow, rawDateTimeStr, pageURL, ColorReset)
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

			if title != "" && body != "" && pageDate != "" && pageTime != "" {
				products = append(products, Data{
					Title: title,
					Body:  body,
					Href:  pageURL,
					Date:  pageDate,
					Time:  pageTime,
				})
				pageStatusMessage = fmt.Sprintf("Успех: %s", LimitString(title, 60))
				statusMessageColor = ColorGreen
				parsedSuccessfully = true
			} else {
				statusMessageColor = ColorYellow
				errMsg := fmt.Sprintf("T:%t|B:%t|D:%t|Ti:%t", title != "", body != "", pageDate != "", pageTime != "")
				pageStatusMessage = fmt.Sprintf("Нет данных [%s]: %s", errMsg, LimitString(pageURL, 30))
			}
		}

		if !parsedSuccessfully && err == nil {
			errItems = append(errItems, fmt.Sprintf("%s (нет данных: T:%t,B:%t,D:%t,Ti:%t)", LimitString(pageURL, 50), title != "", body != "", pageDate != "", pageTime != ""))
		}

		ProgressBar(title, body, pageStatusMessage, statusMessageColor, i, totalLinks)
	}

	if len(products) > 0 {
		fmt.Printf("\n%s[INFO] Парсинг статей DumaTV.ru завершен. Собрано %d статей.%s\n", ColorGreen, len(products), ColorReset)
		if len(errItems) > 0 {
			fmt.Printf("%s[WARNING] Не удалось обработать %d из %d страниц:%s\n", ColorYellow, len(errItems), totalLinks, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	} else if totalLinks > 0 {
		fmt.Printf("\n%s[ERROR] Парсинг статей DumaTV.ru завершен, но не удалось собрать данные ни с одной из %d страниц.%s\n", ColorRed, totalLinks, ColorReset)
		if len(errItems) > 0 {
			fmt.Printf("%s[INFO] Список страниц с ошибками или без данных:%s\n", ColorYellow, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	}

	return products
}
