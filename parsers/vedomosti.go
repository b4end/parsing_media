package parsers

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	. "parsing_media/utils" // Убедитесь, что путь к вашему пакету utils верный

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
)

const (
	vedomostiBaseURL     = "https://www.vedomosti.ru"
	vedomostiNewsPageURL = "https://www.vedomosti.ru/newsline"

	numWorkersVedomosti      = 10 // Можно настроить, как в TASS
	chromedpTimeoutVedomosti = 60 * time.Second
	pageLoadTimeoutVedomosti = 30 * time.Second
)

func VedomostiMain() {
	totalStartTime := time.Now()
	articles := getLinksVedomosti() // Получаем и используем результат
	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("%s[VEDOMOSTI]%s[INFO] Парсер Vedomosti.ru завершил работу. Собрано статей: %d. Время: (%s)%s\n",
		ColorBlue, ColorYellow, len(articles), FormatDuration(totalElapsedTime), ColorReset)
}

func getLinksVedomosti() []Data {
	var foundLinks []string
	seenLinks := make(map[string]bool)
	// Селектор для ссылок на странице ленты: a.news-line__item внутри ul.news-line__list
	linkSelector := `ul.news-line__list a.news-line__item`

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("blink-settings", "imagesEnabled=false"),
		chromedp.Flag("disable-extensions", true),
		chromedp.UserAgent(`Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36`),
	)

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc()

	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	defer cancelBrowser()

	ctx, cancelTimeout := context.WithTimeout(browserCtx, chromedpTimeoutVedomosti)
	defer cancelTimeout()

	var nodes []*cdp.Node
	err := chromedp.Run(ctx,
		chromedp.Navigate(vedomostiNewsPageURL),
		chromedp.WaitVisible(linkSelector, chromedp.ByQuery), // Ждем появления первой ссылки
		chromedp.Nodes(linkSelector, &nodes, chromedp.ByQueryAll),
	)

	if err != nil {
		fmt.Printf("%s[VEDOMOSTI]%s[ERROR] Ошибка при получении ссылок с %s: %v%s\n", ColorBlue, ColorRed, vedomostiNewsPageURL, err, ColorReset)
		return []Data{}
	}

	for _, node := range nodes {
		href := node.AttributeValue("href")
		if href != "" {
			fullURL := href
			// Ссылки на vedomosti.ru относительные от корня
			if strings.HasPrefix(href, "/") {
				fullURL = vedomostiBaseURL + href
			} else if !strings.HasPrefix(href, "http://") && !strings.HasPrefix(href, "https://") {
				// Если это не абсолютный URL и не относительный от корня, пропускаем
				continue
			}

			parsedURL, errUrl := url.Parse(fullURL)
			if errUrl != nil {
				continue
			}

			// Проверяем, что ссылка ведет на vedomosti.ru
			if strings.HasSuffix(parsedURL.Host, "vedomosti.ru") {
				// Исключаем ссылки, которые могут быть не новостями (например, якоря или спецпроекты без нужной структуры)
				// Можно добавить более строгие фильтры по пути, если необходимо
				if !seenLinks[fullURL] {
					seenLinks[fullURL] = true
					foundLinks = append(foundLinks, fullURL)
				}
			}
		}
	}

	if len(foundLinks) == 0 {
		fmt.Printf("%s[VEDOMOSTI]%s[WARNING] Не найдено ссылок с селектором '%s' на странице %s.%s\n", ColorBlue, ColorYellow, linkSelector, vedomostiNewsPageURL, ColorReset)
	} else {
		//fmt.Printf("%s[VEDOMOSTI]%s[INFO] Найдено %d уникальных ссылок на новости.%s\n", ColorBlue, ColorGreen, len(foundLinks), ColorReset)
	}

	return getPageVedomosti(foundLinks, browserCtx)
}

type pageParseResultVedomosti struct {
	Data    Data
	Error   error
	PageURL string
	IsEmpty bool
	Reasons []string
}

// parseVedomostiDate не нужна отдельная функция, т.к. time.Parse справится с RFC3339
// func parseVedomostiDate(dateStr string) (time.Time, error) { ... }

func getPageVedomosti(links []string, parentBrowserCtx context.Context) []Data {
	var articles []Data
	var errItems []string
	totalLinks := len(links)

	if totalLinks == 0 {
		return articles
	}

	// Для Vedomosti дата в UTC+3, time.Parse сам обработает таймзону из строки RFC3339
	// locationPlus3 := time.FixedZone("UTC+3", 3*60*60) // Не обязательно, если парсим RFC3339
	tagsAreMandatory := false // Теги часто бывают "хлебными крошками", сделаем их необязательными

	resultsChan := make(chan pageParseResultVedomosti, totalLinks)
	linkChan := make(chan string, totalLinks)

	for _, link := range links {
		linkChan <- link
	}
	close(linkChan)

	var wg sync.WaitGroup

	actualNumWorkers := numWorkersVedomosti
	if totalLinks < numWorkersVedomosti {
		actualNumWorkers = totalLinks
	}

	for i := 0; i < actualNumWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for pageURL := range linkChan {
				taskCtx, taskCancel := chromedp.NewContext(parentBrowserCtx)
				pageCtxWithTimeout, pageCtxCancel := context.WithTimeout(taskCtx, pageLoadTimeoutVedomosti)

				var title, dateAttrRaw string
				var articlePTexts []string
				var tagTexts []string
				var parsDate time.Time
				var dateParseError error

				titleSelector := `h1.article-headline__title`
				// Тело статьи: параграфы внутри div.article-boxes-list (может быть и другой родитель)
				// Более общий вариант: p.box-paragraph__text
				paragraphSelector := `p.box-paragraph__text`
				// Селектор для даты: атрибут datetime у тега time.article-meta__date
				// Возьмем первый такой элемент, предполагая, что это дата статьи
				dateSelector := `time.article-meta__date`
				// Теги: текст из <a> внутри span.article-meta__tags, исключая "Главная"
				tagsContainerSelector := `div.article-meta` // Контейнер, где искать теги
				// JS для извлечения тегов:
				tagsSelectorJS := `
					Array.from(document.querySelectorAll('div.article-meta span.article-meta__tags a'))
						.map(el => el.innerText.trim())
						.filter(tag => tag.length > 0 && tag.toLowerCase() !== 'главная') 
				`

				actions := []chromedp.Action{
					chromedp.Navigate(pageURL),
					chromedp.WaitVisible(titleSelector, chromedp.ByQuery),
					// Ждем первый параграф, чтобы убедиться, что тело статьи загрузилось
					chromedp.WaitVisible(paragraphSelector, chromedp.ByQuery),
					chromedp.WaitVisible(dateSelector, chromedp.ByQuery),

					chromedp.Text(titleSelector, &title, chromedp.ByQuery),

					chromedp.Evaluate(fmt.Sprintf(`
						Array.from(document.querySelectorAll('%s')).map(el => el.innerText.trim()).filter(p => p.length > 0)
					`, paragraphSelector), &articlePTexts),

					// Получаем атрибут 'datetime'
					chromedp.AttributeValue(dateSelector, "datetime", &dateAttrRaw, nil, chromedp.ByQuery),

					chromedp.ActionFunc(func(ctx context.Context) error {
						var tempTagTexts []string
						// Проверим наличие контейнера тегов (хотя JS-селектор уже достаточно специфичен)
						var tagContainerNodes []*cdp.Node
						errNodes := chromedp.Nodes(tagsContainerSelector, &tagContainerNodes, chromedp.ByQuery, chromedp.AtLeast(0)).Do(ctx)
						if errNodes != nil {
							// Не фатально
						}

						if len(tagContainerNodes) > 0 || tagsContainerSelector == "" { // Если контейнер есть или не указан
							errEval := chromedp.Evaluate(tagsSelectorJS, &tempTagTexts).Do(ctx)
							if errEval != nil {
								// Не фатально
							}
						}
						tagTexts = tempTagTexts
						return nil
					}),
				}

				if err := chromedp.Run(pageCtxWithTimeout, actions...); err != nil {
					errMsg := fmt.Errorf("ошибка chromedp при обработке %s: %w", pageURL, err)
					if pageCtxWithTimeout.Err() == context.DeadlineExceeded {
						errMsg = fmt.Errorf("таймаут (%s) при обработке страницы %s: %w", pageLoadTimeoutVedomosti, pageURL, err)
					}
					resultsChan <- pageParseResultVedomosti{PageURL: pageURL, Error: errMsg}
					pageCtxCancel()
					taskCancel()
					continue
				}

				title = strings.TrimSpace(title)
				var bodyBuilder strings.Builder
				for _, pText := range articlePTexts {
					if bodyBuilder.Len() > 0 {
						bodyBuilder.WriteString("\n\n")
					}
					bodyBuilder.WriteString(pText)
				}
				body := bodyBuilder.String()

				dateAttrRaw = strings.TrimSpace(dateAttrRaw)
				if dateAttrRaw != "" {
					// Пытаемся распарсить дату из атрибута datetime
					// Формат RFC3339: "2006-01-02T15:04:05Z07:00"
					parsedTime, err := time.Parse(time.RFC3339, dateAttrRaw)
					if err != nil {
						dateParseError = fmt.Errorf("ошибка парсинга даты '%s': %w", dateAttrRaw, err)
					} else {
						parsDate = parsedTime
					}
				} else {
					dateParseError = fmt.Errorf("атрибут datetime для даты не найден или пуст")
				}

				pageCtxCancel()
				taskCancel()

				if title != "" && body != "" && !parsDate.IsZero() && (!tagsAreMandatory || len(tagTexts) > 0) {
					resultsChan <- pageParseResultVedomosti{Data: Data{
						Href:  pageURL,
						Title: title,
						Body:  body,
						Date:  parsDate,
						Tags:  tagTexts,
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
							reasonDate = fmt.Sprintf("D:false (err: %v, str: '%s')", dateParseError, dateAttrRaw)
						} else if dateAttrRaw == "" {
							reasonDate = "D:false (empty_attr_str)"
						}
						reasons = append(reasons, reasonDate)
					}
					if tagsAreMandatory && len(tagTexts) == 0 {
						reasons = append(reasons, "Tags:false")
					}
					resultsChan <- pageParseResultVedomosti{PageURL: pageURL, IsEmpty: true, Reasons: reasons}
				}
			}
		}(i)
	}

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	processedCount := 0
	for result := range resultsChan {
		processedCount++
		if result.Error != nil {
			errItems = append(errItems, fmt.Sprintf("%s (%s)", result.PageURL, result.Error.Error()))
		} else if result.IsEmpty {
			// Можно логировать подробно, если нужно
			// fmt.Printf("%s[VEDOMOSTI]%s[DEBUG] Страница без данных: %s (причины: %s)%s\n", ColorBlue, ColorCyan, result.PageURL, strings.Join(result.Reasons, ", "), ColorReset)
		} else {
			articles = append(articles, result.Data)
		}
	}

	if len(articles) > 0 {
		if len(errItems) > 0 {
			fmt.Printf("%s[VEDOMOSTI]%s[WARNING] Не удалось обработать или отсутствовали критичные данные для %d из %d страниц:%s\n", ColorBlue, ColorYellow, len(errItems), totalLinks, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, LimitString(itemMessage, 250), ColorReset)
			}
		}
	} else if totalLinks > 0 {
		fmt.Printf("%s[VEDOMOSTI]%s[ERROR] Парсинг статей Vedomosti.ru завершен, но не удалось собрать данные ни с одной из %d страниц.%s\n", ColorBlue, ColorRed, totalLinks, ColorReset)
		if len(errItems) > 0 {
			fmt.Printf("%s[VEDOMOSTI]%s[INFO] Список страниц с ошибками или без данных:%s\n", ColorBlue, ColorYellow, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, LimitString(itemMessage, 250), ColorReset)
			}
		}
	}
	return articles
}
