package parsers

import (
	"fmt"
	. "parsing_media/utils"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	kommersURL     = "https://www.kommersant.ru"
	kommersURLNews = "https://www.kommersant.ru/lenta"
)

// LinkItem будет хранить URL статьи и предварительно собранные теги
type LinkItem struct {
	Href string
	Tags []string
}

func KommersMain() {
	totalStartTime := time.Now()

	_ = getLinksKommers()

	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("%s[KOMMERSANT]%s[INFO] Парсер Kommersant.ru заверщил работу: (%s)%s\n", ColorBlue, ColorYellow, FormatDuration(totalElapsedTime), ColorReset)
}

func getLinksKommers() []Data {
	var foundLinkItems []LinkItem
	seenLinks := make(map[string]bool)

	doc, err := GetHTML(kommersURLNews)
	if err != nil {
		fmt.Printf("%s[KOMMERSANT]%s[ERROR] Ошибка при получении HTML со страницы %s: %v%s\n", ColorBlue, ColorRed, kommersURLNews, err, ColorReset)
		return nil
	}

	articleSelector := "article.uho.rubric_lenta__item.js-article"
	linkSelector := "h2.uho__name a.uho__link--overlay"
	tagSelector := "ul.crumbs.tag_list li.tag_list__item a.tag_list__link"

	doc.Find(articleSelector).Each(func(i int, articleSelection *goquery.Selection) {
		var articleHref string
		var articleTags []string

		href, exists := articleSelection.Find(linkSelector).Attr("href")
		if exists {
			fullHref := ""
			if strings.HasPrefix(href, "/") {
				fullHref = kommersURL + href
			} else if strings.HasPrefix(href, kommersURL) {
				fullHref = href
			}
			if fullHref != "" {
				articleHref = fullHref
			}
		}

		if articleHref == "" {
			return
		}

		articleSelection.Find(tagSelector).Each(func(_ int, tagLink *goquery.Selection) {
			tagText := strings.TrimSpace(tagLink.Text())
			if tagText != "" {
				articleTags = append(articleTags, tagText)
			}
		})

		if !seenLinks[articleHref] && len(articleTags) > 0 {
			seenLinks[articleHref] = true
			foundLinkItems = append(foundLinkItems, LinkItem{
				Href: articleHref,
				Tags: articleTags,
			})
		} else if !seenLinks[articleHref] && len(articleTags) == 0 {
		}
	})

	if len(foundLinkItems) == 0 {
		fmt.Printf("%s[KOMMERSANT]%s[WARNING] Не найдено ссылок с тегами на странице %s (селектор статьи: '%s').%s\n", ColorBlue, ColorYellow, kommersURLNews, articleSelector, ColorReset)
	}

	return getPageKommers(foundLinkItems)
}

func getPageKommers(linkItems []LinkItem) []Data {
	var products []Data
	var errItems []string
	totalLinks := len(linkItems)

	if totalLinks == 0 {
		return products
	}

	tagsAreMandatoryForThisParser := true

	for _, item := range linkItems {
		pageURL := item.Href
		preloadedTags := item.Tags

		var title, body string
		var parsDate time.Time
		parsedSuccessfully := false

		doc, err := GetHTML(pageURL)
		if err != nil {
			errItems = append(errItems, fmt.Sprintf("(ошибка GET: %s)", err.Error()))
		} else {
			title = strings.TrimSpace(doc.Find("h1.doc_header__name.js-search-mark").First().Text())

			var bodyBuilder strings.Builder
			doc.Find("div.article_text_wrapper.js-search-mark p.doc__text").Not(".document_authors").Each(func(_ int, s *goquery.Selection) {
				paragraphText := strings.TrimSpace(s.Text())
				if strings.Contains(paragraphText, "Материал дополняется") ||
					strings.HasPrefix(paragraphText, "Читайте также:") ||
					strings.HasPrefix(paragraphText, "Фото:") ||
					paragraphText == "" {
					return
				}
				if bodyBuilder.Len() > 0 {
					bodyBuilder.WriteString("\n\n")
				}
				bodyBuilder.WriteString(paragraphText)
			})
			body = bodyBuilder.String()

			dateSelector := `time.doc_header__publish_time`
			dateStr, exists := doc.Find(dateSelector).Attr("datetime")
			if exists {
				parsedTime, err := time.Parse(time.RFC3339, dateStr)
				if err != nil {
					fmt.Printf("%s[KOMMERSANT]%s[WARNING] Ошибка парсинга даты: '%s' (селектор: '%s') на %s: %v%s\n", ColorBlue, ColorYellow, dateStr, dateSelector, pageURL, err, ColorReset)
				} else {
					parsDate = parsedTime
				}
			} else {
				fmt.Printf("%s[KOMMERSANT]%s[INFO] Атрибут 'datetime' с датой не найден (селектор: '%s') на %s%s\n", ColorBlue, ColorYellow, dateSelector, pageURL, ColorReset)
			}

			allMandatoryFieldsPresent := title != "" && body != "" && !parsDate.IsZero()
			if tagsAreMandatoryForThisParser {
				allMandatoryFieldsPresent = allMandatoryFieldsPresent && len(preloadedTags) > 0
			}

			if allMandatoryFieldsPresent {
				products = append(products, Data{
					Href:  pageURL,
					Title: title,
					Body:  body,
					Date:  parsDate,
					Tags:  preloadedTags,
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
			if tagsAreMandatoryForThisParser && len(preloadedTags) == 0 {
				reasons = append(reasons, "Tags:false(mandatory_on_feed)")
			}
			errItems = append(errItems, fmt.Sprintf("%s (нет данных: %s)", pageURL, strings.Join(reasons, ", ")))
		}
	}

	if len(products) > 0 {
		if len(errItems) > 0 {
			fmt.Printf("%s[KOMMERSANT]%s[WARNING] Не удалось обработать %d из %d страниц (или отсутствовали данные):%s\n", ColorBlue, ColorYellow, len(errItems), totalLinks, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	} else if totalLinks > 0 {
		fmt.Printf("%s[KOMMERSANT]%s[ERROR] Парсинг статей Kommersant.ru завершен, но не удалось собрать данные ни с одной из %d страниц.%s\n", ColorBlue, ColorRed, totalLinks, ColorReset)
		if len(errItems) > 0 {
			fmt.Printf("%s[KOMMERSANT]%s[INFO] Список страниц с ошибками или без данных:%s\n", ColorBlue, ColorYellow, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	}
	return products
}
