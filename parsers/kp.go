package parsers

import (
	"encoding/json"
	"fmt"
	"net/http"
	. "parsing_media/utils"
	"regexp"  // Понадобится для извлечения даты из строки "X минут/час/часов назад"
	"strconv" // Для преобразования чисел из строки
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	kpURL         = "https://www.kp.ru"
	kpNewsPageURL = "https://www.kp.ru/online/" // Убедимся, что URL правильный
	numWorkersKP  = 10
)

func KPMain() {
	totalStartTime := time.Now()
	_ = getLinksKP()
	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("%s[KP]%s[INFO] Парсер KP.ru завершил работу: (%s)%s\n", ColorBlue, ColorYellow, FormatDuration(totalElapsedTime), ColorReset)
}

func getLinksKP() []Data {
	var foundLinks []string
	seenLinks := make(map[string]bool)
	// Селектор для ссылок на статьи в ленте новостей
	// Нацеливаемся на <a class="sc-1tputnk-2 drlShK"> или <a class="sc-1tputnk-3 dcIDGO">
	// Оба они содержат href="/online/news/ID/"
	linkSelector := "div.sc-lvle83-0 a[href^='/online/news/']"

	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	doc, err := GetHTMLForClient(client, kpNewsPageURL)
	if err != nil {
		fmt.Printf("%s[KP]%s[ERROR] Ошибка при получении HTML со страницы %s: %v%s\n", ColorBlue, ColorRed, kpNewsPageURL, err, ColorReset)
		return getPageKP(foundLinks)
	}

	doc.Find(linkSelector).Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists {
			// Убедимся, что это действительно ссылка на новость, а не на рубрику
			// Ссылки на новости имеют вид /online/news/ID/
			if strings.HasPrefix(href, "/online/news/") && strings.Count(strings.Trim(href, "/"), "/") == 2 {
				fullURL := kpURL + href

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
		fmt.Printf("%s[KP]%s[WARNING] Не найдено ссылок с селектором '%s' на странице %s.%s\n", ColorBlue, ColorYellow, linkSelector, kpNewsPageURL, ColorReset)
	}

	return getPageKP(foundLinks)
}

type pageParseResultKP struct {
	Data    Data
	Error   error
	PageURL string
	IsEmpty bool
	Reasons []string
}

// parseRelativeTime преобразует строки типа "X минут/час/часов назад" в time.Time
func parseRelativeTimeKP(timeStr string) (time.Time, error) {
	now := time.Now()

	reMinute := regexp.MustCompile(`(\d+)\s+(минут[аы]?|мин\.?)\s+назад`)
	reHour := regexp.MustCompile(`(\d+)\s+(час[аов]?|ч\.?)\s+назад`)

	if matches := reMinute.FindStringSubmatch(timeStr); len(matches) > 1 {
		minutes, err := strconv.Atoi(matches[1])
		if err != nil {
			return time.Time{}, fmt.Errorf("не удалось преобразовать минуты '%s': %w", matches[1], err)
		}
		return now.Add(-time.Duration(minutes) * time.Minute), nil
	}

	if matches := reHour.FindStringSubmatch(timeStr); len(matches) > 1 {
		hours, err := strconv.Atoi(matches[1])
		if err != nil {
			return time.Time{}, fmt.Errorf("не удалось преобразовать часы '%s': %w", matches[1], err)
		}
		return now.Add(-time.Duration(hours) * time.Hour), nil
	}

	// Если это не относительное время, пытаемся парсить как абсолютное "DD MMMM YYYY HH:MM"
	// Пример: "4 июня 2025 19:13"
	// Сначала заменим русские месяцы
	tempDateStr := timeStr
	for rusM, engMNum := range RussianMonths {
		tempDateStr = strings.ReplaceAll(tempDateStr, rusM, engMNum)
	}
	// Формат "2 01 2006 15:04"
	layoutAbsolute := "2 01 2006 15:04"
	locationMSK := time.FixedZone("MSK", 3*60*60) // КП обычно в MSK
	parsedTime, err := time.ParseInLocation(layoutAbsolute, tempDateStr, locationMSK)
	if err == nil {
		return parsedTime, nil
	}

	return time.Time{}, fmt.Errorf("не удалось распознать формат времени: '%s'", timeStr)
}

func getPageKP(links []string) []Data {
	var products []Data
	var errItems []string
	totalLinks := len(links)

	if totalLinks == 0 {
		return products
	}

	locationMSK := time.FixedZone("MSK", 3*60*60) // КП обычно в MSK
	tagsAreMandatory := false

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: numWorkersKP + 5,
			IdleConnTimeout:     90 * time.Second,
			MaxConnsPerHost:     numWorkersKP,
		},
	}

	resultsChan := make(chan pageParseResultKP, totalLinks)
	linkChan := make(chan string, totalLinks)

	for _, link := range links {
		linkChan <- link
	}
	close(linkChan)

	var wg sync.WaitGroup

	actualNumWorkers := numWorkersKP
	if totalLinks < numWorkersKP {
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
					resultsChan <- pageParseResultKP{PageURL: pageURL, Error: fmt.Errorf("ошибка GET: %w", err)}
					continue
				}

				title = strings.TrimSpace(doc.Find("h1.sc-j7em19-3.eyeguj").First().Text())

				var bodyBuilder strings.Builder
				// div[data-gtm-el="content-body"] p.sc-1wayp1z-16
				doc.Find("div[data-gtm-el='content-body'] p.sc-1wayp1z-16").Each(func(j int, s *goquery.Selection) {
					// Исключаем <p> внутри рекламных/промо блоков
					if s.Closest("div.sc-1tputnk-12.cizwKg.sc-14w6ld7-0.hKabcu").Length() > 0 { // Промо-блок из примера
						return
					}
					if s.Closest("div[data-name='10.1m']").Length() > 0 { // Рекламный блок
						return
					}

					currentTextPart := strings.TrimSpace(s.Text())
					if currentTextPart != "" {
						if bodyBuilder.Len() > 0 {
							bodyBuilder.WriteString("\n\n")
						}
						bodyBuilder.WriteString(currentTextPart)
					}
				})
				body = bodyBuilder.String()

				// Дата на kp.ru часто бывает в формате "X минут/час назад" или "DD MMMM YYYY HH:MM"
				// <span class="sc-j7em19-1 dtkLMY">4 июня 2025 19:13</span>
				dateTextRaw := strings.TrimSpace(doc.Find("span.sc-j7em19-1.dtkLMY").First().Text())
				if dateTextRaw == "" { // Попытка найти дату в ленте, если на странице статьи ее нет в явном виде (маловероятно для КП)
					// Это скорее для времени типа "час назад" которое может быть в ленте
					dateTextRaw = strings.TrimSpace(doc.Find("span.sc-1tputnk-9.gpa-DyG").First().Text())
				}

				if dateTextRaw != "" {
					parsedTime, parseErr := parseRelativeTimeKP(dateTextRaw)
					if parseErr != nil {
						// Дополнительная попытка извлечь из JSON-LD, если он есть
						doc.Find("script[type='application/ld+json']").EachWithBreak(func(_ int, sNode *goquery.Selection) bool {
							var jsonData map[string]interface{}
							if err := json.Unmarshal([]byte(sNode.Text()), &jsonData); err == nil {
								if datePublished, ok := jsonData["datePublished"].(string); ok {
									pt, errLd := time.Parse(time.RFC3339, datePublished)
									if errLd == nil {
										parsDate = pt.In(locationMSK)
										return false // Прерываем цикл, дата найдена
									}
								}
							}
							return true
						})
						if parsDate.IsZero() { // Если JSON-LD не помог
							dateParseError = fmt.Errorf("ошибка парсинга даты '%s': %v", dateTextRaw, parseErr)
						}
					} else {
						parsDate = parsedTime.In(locationMSK) // Убедимся, что относительное время тоже в MSK
					}
				} else {
					dateParseError = fmt.Errorf("строка даты не найдена")
				}

				if !parsDate.IsZero() {
					parsDate = parsDate.In(locationMSK)
					dateParseError = nil
				}

				if dateParseError != nil && parsDate.IsZero() {
					fmt.Printf("%s[KP]%s[WARNING] Ошибка парсинга даты: '%s' на %s: %v%s\n", ColorBlue, ColorYellow, dateTextRaw, pageURL, dateParseError, ColorReset)
				}

				// Теги: <div class="sc-j7em19-2 dQphFo"><a class="sc-1vxg2pp-0 cXMtmu">Тег</a>...</div>
				// Первый элемент в этом блоке обычно "Новости", его можно пропустить, если это нежелательный тег.
				// Остальные - рубрики/темы.
				doc.Find("div.sc-j7em19-2.dQphFo a.sc-1vxg2pp-0.cXMtmu").Each(func(i int, s *goquery.Selection) {
					// Пропускаем первый элемент "Новости", если он всегда там и не нужен как тег
					// if i == 0 && strings.ToLower(strings.TrimSpace(s.Text())) == "новости" {
					// 	return
					// }
					tagText := strings.TrimSpace(s.Text())
					if tagText != "" {
						tags = append(tags, tagText)
					}
				})
				// Альтернативный поиск тегов, если они есть в другом месте (пример, если на странице будут явные теги статьи)
				// doc.Find(".article-tags a, .tags-list a").Each(...)

				if len(tags) > 0 {
					seenTags := make(map[string]bool)
					uniqueTags := []string{}
					for _, tag := range tags {
						if !seenTags[tag] {
							seenTags[tag] = true
							uniqueTags = append(uniqueTags, tag)
						}
					}
					tags = uniqueTags
				}

				if title != "" && body != "" && !parsDate.IsZero() && (!tagsAreMandatory || len(tags) > 0) {
					resultsChan <- pageParseResultKP{Data: Data{
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
							reasonDate = fmt.Sprintf("D:false (err: %v, str: '%s')", dateParseError, dateTextRaw)
						} else if dateTextRaw == "" {
							reasonDate = "D:false (empty_str)"
						}
						reasons = append(reasons, reasonDate)
					}
					if tagsAreMandatory && len(tags) == 0 {
						reasons = append(reasons, "Tags:false")
					}
					resultsChan <- pageParseResultKP{PageURL: pageURL, IsEmpty: true, Reasons: reasons}
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
			fmt.Printf("%s[KP]%s[WARNING] Не удалось обработать %d из %d страниц (или отсутствовали данные):%s\n", ColorBlue, ColorYellow, len(errItems), totalLinks, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	} else if totalLinks > 0 {
		fmt.Printf("%s[KP]%s[ERROR] Парсинг статей KP.ru завершен, но не удалось собрать данные ни с одной из %d страниц.%s\n", ColorBlue, ColorRed, totalLinks, ColorReset)
		if len(errItems) > 0 {
			fmt.Printf("%s[KP]%s[INFO] Список страниц с ошибками или без данных:%s\n", ColorBlue, ColorYellow, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	}
	return products
}
