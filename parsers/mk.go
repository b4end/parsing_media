package parsers

import (
	"fmt"
	. "parsing_media/utils"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	mkURL         = "https://www.mk.ru"
	mkNewsPageURL = "https://www.mk.ru/news/"
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

	doc, err := GetHTML(targetURL)
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

func getPageMK(links []string) []Data {
	var products []Data
	var errItems []string
	totalLinks := len(links)

	if totalLinks == 0 {
		return products
	}

	const mkDateLayout = "2006-01-02T15:04:05-0700"

	for _, pageURL := range links {
		var title, body string
		var parsDate time.Time
		var tags []string
		parsedSuccessfully := false

		doc, err := GetHTML(pageURL)
		if err != nil {
			errItems = append(errItems, fmt.Sprintf("(ошибка GET: %s)", err.Error()))
		} else {
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
				}
			}

			if title != "" && body != "" && !parsDate.IsZero() {
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
			errItems = append(errItems, fmt.Sprintf("%s (нет данных: %s)", pageURL, strings.Join(reasons, ", ")))
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
