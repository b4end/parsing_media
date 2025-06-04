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
	interfaxURL         = "https://www.interfax.ru"
	interfaxNewsPageURL = "https://www.interfax.ru/news/" // Добавил слеш в конце, как в ссылке
	numWorkersInterfax  = 10
)

func InterfaxMain() {
	totalStartTime := time.Now()
	_ = getLinksInterfax()
	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("%s[INTERFAX]%s[INFO] Парсер Interfax.ru завершил работу: (%s)%s\n", ColorBlue, ColorYellow, FormatDuration(totalElapsedTime), ColorReset)
}

func getLinksInterfax() []Data {
	var foundLinks []string
	seenLinks := make(map[string]bool)
	// Селектор для ссылок внутри блока с классом "an", затем div, затем a
	linkSelector := "div.an > div > a"

	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	doc, err := GetHTMLForClient(client, interfaxNewsPageURL)
	if err != nil {
		fmt.Printf("%s[INTERFAX]%s[ERROR] Ошибка при получении HTML со страницы %s: %v%s\n", ColorBlue, ColorRed, interfaxNewsPageURL, err, ColorReset)
		return getPageInterfax(foundLinks)
	}

	doc.Find(linkSelector).Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists {
			fullURL := ""
			if strings.HasPrefix(href, "/") {
				// Проверяем, не является ли это ссылкой на другой поддомен interfax, который мы не хотим парсить (например, sport-interfax.ru)
				// Мы хотим только те, что ведут на основной сайт www.interfax.ru
				if !strings.HasPrefix(href, "//") && !strings.Contains(href, "sport-interfax.ru") && !strings.Contains(href, "realty.interfax.ru") /* можно добавить другие поддомены для исключения */ {
					fullURL = interfaxURL + href
				}
			} else if strings.HasPrefix(href, interfaxURL+"/") { // Явно проверяем основной домен
				fullURL = href
			}
			// Игнорируем ссылки, которые ведут на sport-interfax.ru из HTML примера
			if strings.HasPrefix(href, "https://www.sport-interfax.ru") {
				fullURL = "" // Обнуляем, чтобы не добавлять
			}

			if fullURL != "" {
				if idx := strings.Index(fullURL, "?"); idx != -1 {
					fullURL = fullURL[:idx]
				}
				if !seenLinks[fullURL] {
					seenLinks[fullURL] = true
					foundLinks = append(foundLinks, fullURL)
				}
			}
		}
	})

	if len(foundLinks) == 0 {
		fmt.Printf("%s[INTERFAX]%s[WARNING] Не найдено ссылок с селектором '%s' на странице %s.%s\n", ColorBlue, ColorYellow, linkSelector, interfaxNewsPageURL, ColorReset)
	}

	return getPageInterfax(foundLinks)
}

type pageParseResultInterfax struct {
	Data    Data
	Error   error
	PageURL string
	IsEmpty bool
	Reasons []string
}

func getPageInterfax(links []string) []Data {
	var products []Data
	var errItems []string
	totalLinks := len(links)

	if totalLinks == 0 {
		return products
	}

	// Пример формата из <meta itemprop="datePublished" content="2025-06-04T17:14:00"> - похоже на RFC3339 без смещения/Z
	// Текстовый формат: "17:15, 4 июня 2025"
	tagsAreMandatory := false

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: numWorkersInterfax + 5,
			IdleConnTimeout:     90 * time.Second,
			MaxConnsPerHost:     numWorkersInterfax,
		},
	}

	resultsChan := make(chan pageParseResultInterfax, totalLinks)
	linkChan := make(chan string, totalLinks)

	for _, link := range links {
		linkChan <- link
	}
	close(linkChan)

	var wg sync.WaitGroup

	actualNumWorkers := numWorkersInterfax
	if totalLinks < numWorkersInterfax {
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
					resultsChan <- pageParseResultInterfax{PageURL: pageURL, Error: fmt.Errorf("ошибка GET: %w", err)}
					continue
				}

				title = strings.TrimSpace(doc.Find("article[itemprop='articleBody'] h1[itemprop='headline']").First().Text())

				var bodyBuilder strings.Builder
				doc.Find("article[itemprop='articleBody'] p").Each(func(j int, s *goquery.Selection) {
					// Исключаем пустые <p> или <p> только с <br>
					if strings.TrimSpace(s.Text()) == "" && s.Find("br").Length() > 0 && s.Children().Length() == s.Find("br").Length() {
						return
					}
					// Исключаем абзацы, которые могут быть подписями к фото или встроенной рекламой, если они имеют специфичные классы или структуру
					// (пока не видно таких в примере, но можно добавить при необходимости)

					currentTextPart := strings.TrimSpace(s.Text())
					if currentTextPart != "" {
						if bodyBuilder.Len() > 0 {
							bodyBuilder.WriteString("\n\n")
						}
						bodyBuilder.WriteString(currentTextPart)
					}
				})
				body = bodyBuilder.String()

				dateTextRaw := ""
				// Приоритет отдаем meta тегу
				metaDate, metaDateExists := doc.Find("meta[itemprop='datePublished']").First().Attr("content")
				if metaDateExists {
					dateTextRaw = metaDate // "2025-06-04T17:14:00"
				} else {
					// Если meta тега нет, пытаемся взять из <time>
					timeText, timeTextExists := doc.Find("time[datetime]").First().Attr("datetime") // ищем атрибут datetime
					if timeTextExists {
						dateTextRaw = timeText
					} else { // если нет datetime, берем текст из ссылки внутри time
						dateTextRaw = strings.TrimSpace(doc.Find("time a.time").First().Text()) // "17:15, 4 июня 2025"
					}
				}
				dateToParse := strings.TrimSpace(dateTextRaw)

				locationMSK := time.FixedZone("MSK", 3*60*60) // Интерфакс обычно в MSK

				if dateToParse != "" {
					// Попытка 1: RFC3339 или похожий формат без явного смещения (предполагаем MSK)
					// "2025-06-04T17:14:00"
					layoutRFC := "2006-01-02T15:04:05" // Этот формат подходит для "2025-06-04T17:14:00"
					parsedTime, parseErr := time.ParseInLocation(layoutRFC, dateToParse, locationMSK)

					if parseErr == nil {
						parsDate = parsedTime
					} else {
						// Попытка 2: для формата "17:15, 4 июня 2025"
						tempDateStr := dateToParse
						for rusM, engMNum := range RussianMonths {
							tempDateStr = strings.ReplaceAll(tempDateStr, rusM, engMNum)
						}
						// После замены: "17:15, 4 06 2025"
						layoutCustom := "15:04, 2 01 2006" // Час:Минута, Число Месяц Год

						parsedTimeCustom, parseErrCustom := time.ParseInLocation(layoutCustom, tempDateStr, locationMSK)
						if parseErrCustom != nil {
							dateParseError = fmt.Errorf("ошибка парсинга даты '%s' (RFC_like_err: %v, Custom_err: %v)", dateToParse, parseErr, parseErrCustom)
						} else {
							parsDate = parsedTimeCustom
						}
					}
				} else {
					dateParseError = fmt.Errorf("строка даты пуста")
				}

				if !parsDate.IsZero() { // Если дата успешно спарсена
					parsDate = parsDate.In(locationMSK) // Убедимся, что она точно в MSK
					dateParseError = nil                // Сбрасываем ошибку, если дата в итоге есть
				}

				if dateParseError != nil && parsDate.IsZero() {
					fmt.Printf("%s[INTERFAX]%s[WARNING] Ошибка парсинга даты: '%s' на %s: %v%s\n", ColorBlue, ColorYellow, dateToParse, pageURL, dateParseError, ColorReset)
				}

				doc.Find(".textMTags a").Each(func(_ int, s *goquery.Selection) {
					tagText := strings.TrimSpace(s.Text())
					if tagText != "" {
						tags = append(tags, tagText)
					}
				})

				if title != "" && body != "" && !parsDate.IsZero() && (!tagsAreMandatory || len(tags) > 0) {
					resultsChan <- pageParseResultInterfax{Data: Data{
						Href:  pageURL,
						Title: title,
						Body:  body,
						Date:  parsDate,
						Tags:  tags,
					}}
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
					resultsChan <- pageParseResultInterfax{PageURL: pageURL, IsEmpty: true, Reasons: reasons}
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
			fmt.Printf("%s[INTERFAX]%s[WARNING] Не удалось обработать %d из %d страниц (или отсутствовали данные):%s\n", ColorBlue, ColorYellow, len(errItems), totalLinks, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	} else if totalLinks > 0 {
		fmt.Printf("%s[INTERFAX]%s[ERROR] Парсинг статей Interfax.ru завершен, но не удалось собрать данные ни с одной из %d страниц.%s\n", ColorBlue, ColorRed, totalLinks, ColorReset)
		if len(errItems) > 0 {
			fmt.Printf("%s[INTERFAX]%s[INFO] Список страниц с ошибками или без данных:%s\n", ColorBlue, ColorYellow, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	}
	return products
}
