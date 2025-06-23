package parsers

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	. "parsing_media/utils"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
)

const (
	tassBaseURL     = "https://tass.ru"
	tassNewsPageURL = "https://tass.ru/novosti-dnya"
	numWorkersTass  = 5
	chromedpTimeout = 90 * time.Second
	pageLoadTimeout = 60 * time.Second
)

func TassMain() {
	totalStartTime := time.Now()
	_ = getLinksTass()
	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("%s[TASS]%s[INFO] Парсер TASS.ru заверщил работу: (%s)%s\n", ColorBlue, ColorYellow, FormatDuration(totalElapsedTime), ColorReset)
}

func getLinksTass() []Data {
	var foundLinks []string
	seenLinks := make(map[string]bool)
	linkSelector := `div#infinite_listing a.tass_pkg_link-v5WdK`

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

	ctx, cancelTimeout := context.WithTimeout(browserCtx, chromedpTimeout)
	defer cancelTimeout()

	var nodes []*cdp.Node
	err := chromedp.Run(ctx,
		chromedp.Navigate(tassNewsPageURL),
		chromedp.WaitVisible(linkSelector, chromedp.ByQuery),
		chromedp.Nodes(linkSelector, &nodes, chromedp.ByQueryAll),
	)

	if err != nil {
		fmt.Printf("%s[TASS]%s[ERROR] Ошибка при получении ссылок с %s: %v%s\n", ColorBlue, ColorRed, tassNewsPageURL, err, ColorReset)
		return []Data{}
	}

	for _, node := range nodes {
		href := node.AttributeValue("href")
		if href != "" {
			fullURL := href
			if strings.HasPrefix(href, "/") {
				fullURL = tassBaseURL + href
			} else if !strings.HasPrefix(href, "http://") && !strings.HasPrefix(href, "https://") {
				continue
			}

			parsedURL, errUrl := url.Parse(fullURL)
			if errUrl != nil {
				continue
			}

			if strings.HasSuffix(parsedURL.Host, "tass.ru") {
				if !seenLinks[fullURL] {
					seenLinks[fullURL] = true
					foundLinks = append(foundLinks, fullURL)
				}
			}
		}
	}

	if len(foundLinks) == 0 {
		fmt.Printf("%s[TASS]%s[WARNING] Не найдено ссылок с селектором '%s' на странице %s.%s\n", ColorBlue, ColorYellow, linkSelector, tassNewsPageURL, ColorReset)
	}

	return getPageTass(foundLinks, browserCtx)
}

type pageParseResultTass struct {
	Data    Data
	Error   error
	PageURL string
	IsEmpty bool
	Reasons []string
}

func parseTassDate(dateStr string, location *time.Location, russianMonths map[string]string) (time.Time, error) {
	dateStr = strings.TrimSpace(dateStr)
	dateStr = strings.ReplaceAll(dateStr, "\u00a0", " ")

	parts := strings.FieldsFunc(dateStr, func(r rune) bool {
		return r == ' ' || r == ','
	})

	var day, monthWord, year, timeHM string

	if len(parts) < 4 {
		return time.Time{}, fmt.Errorf("недостаточно частей в строке даты '%s' (%d частей: %v)", dateStr, len(parts), parts)
	}

	day = parts[0]
	monthWord = strings.ToLower(parts[1])
	year = parts[2]

	foundTime := false
	for i := 3; i < len(parts); i++ {
		timeParts := strings.Split(parts[i], ":")
		if len(timeParts) == 2 {
			if _, err1 := fmt.Sscan(timeParts[0], new(int)); err1 == nil {
				if _, err2 := fmt.Sscan(timeParts[1], new(int)); err2 == nil {
					timeHM = parts[i]
					foundTime = true
					break
				}
			}
		}
	}
	if !foundTime {
		return time.Time{}, fmt.Errorf("не удалось найти время (HH:MM) в строке даты: '%s' (части: %v)", dateStr, parts)
	}

	monthNum, ok := russianMonths[monthWord]
	if !ok {
		return time.Time{}, fmt.Errorf("неизвестное название месяца '%s' в строке '%s'", monthWord, dateStr)
	}

	parseableDateStr := fmt.Sprintf("%s %s %s, %s", day, monthNum, year, timeHM)
	layout := "2 01 2006, 15:04"

	t, err := time.ParseInLocation(layout, parseableDateStr, location)
	if err != nil {
		return time.Time{}, fmt.Errorf("ошибка парсинга даты '%s' (исходная '%s', layout '%s'): %w", parseableDateStr, dateStr, layout, err)
	}
	return t, nil
}

func getPageTass(links []string, parentBrowserCtx context.Context) []Data {
	var articles []Data
	var errItems []string
	totalLinks := len(links)

	if totalLinks == 0 {
		return articles
	}

	locationPlus3 := time.FixedZone("UTC+3", 3*60*60)
	tagsAreMandatory := true

	resultsChan := make(chan pageParseResultTass, totalLinks)
	linkChan := make(chan string, totalLinks)

	for _, link := range links {
		linkChan <- link
	}
	close(linkChan)

	var wg sync.WaitGroup

	actualNumWorkers := numWorkersTass
	if totalLinks < numWorkersTass {
		actualNumWorkers = totalLinks
	}

	for i := 0; i < actualNumWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for pageURL := range linkChan {
				taskCtx, taskCancel := chromedp.NewContext(parentBrowserCtx)
				pageCtxWithTimeout, pageCtxCancel := context.WithTimeout(taskCtx, pageLoadTimeout)

				var title, dateTextRaw string
				var articlePTexts []string
				var tagTexts []string
				var parsDate time.Time
				var dateParseError error

				titleSelector := `h1.NewsHeader_titles__uKY5F`
				articleBodySelector := `article.Content_wrapper__DiAVL`
				paragraphSelector := `p.Paragraph_paragraph__9WAFK`
				dateSelector := `div.PublishedMark_date__LG42P`
				tagsContainerSelector := `div.Tags_container__wP7Lb`
				tagsSelectorJS := `Array.from(document.querySelectorAll('div.Tags_container__wP7Lb a.Tags_tag__o7Dqc')).map(el => el.innerText.trim()).filter(tag => tag.length > 0)`

				actions := []chromedp.Action{
					chromedp.Navigate(pageURL),
					chromedp.WaitVisible(titleSelector, chromedp.ByQuery),
					chromedp.WaitVisible(articleBodySelector, chromedp.ByQuery),
					chromedp.WaitVisible(dateSelector, chromedp.ByQuery),
					chromedp.Text(titleSelector, &title, chromedp.ByQuery),
					chromedp.Evaluate(fmt.Sprintf(`
						Array.from(document.querySelectorAll('%s %s')).map(el => el.innerText.trim()).filter(p => p.length > 0)
					`, articleBodySelector, paragraphSelector), &articlePTexts),
					chromedp.Text(dateSelector, &dateTextRaw, chromedp.ByQuery),
					chromedp.ActionFunc(func(ctx context.Context) error {
						var tempTagTexts []string
						var tagContainerNodes []*cdp.Node
						errNodes := chromedp.Nodes(tagsContainerSelector, &tagContainerNodes, chromedp.ByQuery, chromedp.AtLeast(0)).Do(ctx)
						if errNodes != nil {
						}

						if len(tagContainerNodes) > 0 {
							errEval := chromedp.Evaluate(tagsSelectorJS, &tempTagTexts).Do(ctx)
							if errEval != nil {
							}
						}
						tagTexts = tempTagTexts
						return nil
					}),
				}

				if err := chromedp.Run(pageCtxWithTimeout, actions...); err != nil {
					errMsg := fmt.Errorf("ошибка chromedp при обработке %s: %w", pageURL, err)
					if pageCtxWithTimeout.Err() == context.DeadlineExceeded {
						errMsg = fmt.Errorf("таймаут (%s) при обработке страницы %s: %w", pageLoadTimeout, pageURL, err)
					}
					resultsChan <- pageParseResultTass{PageURL: pageURL, Error: errMsg}
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

				dateTextRaw = strings.TrimSpace(dateTextRaw)
				if dateTextRaw != "" {
					parsedTime, err := parseTassDate(dateTextRaw, locationPlus3, RussianMonths)
					if err != nil {
						dateParseError = err
					} else {
						parsDate = parsedTime
					}
				}

				pageCtxCancel()
				taskCancel()

				if title != "" && body != "" && !parsDate.IsZero() && (!tagsAreMandatory || len(tagTexts) > 0) {
					dataItem := Data{
						Site:  tassBaseURL,
						Href:  pageURL,
						Title: title,
						Body:  body,
						Date:  parsDate,
						Tags:  tagTexts,
					}
					hash, err := dataItem.Hashing()
					if err != nil {
						resultsChan <- pageParseResultTass{PageURL: pageURL, Error: fmt.Errorf("ошибка генерации хеша: %w", err)}
					} else {
						dataItem.Hash = hash
						resultsChan <- pageParseResultTass{Data: dataItem}
					}
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
					if tagsAreMandatory && len(tagTexts) == 0 {
						reasons = append(reasons, "Tags:false")
					}
					resultsChan <- pageParseResultTass{PageURL: pageURL, IsEmpty: true, Reasons: reasons}
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
		} else {
			articles = append(articles, result.Data)
		}
	}

	if len(articles) > 0 {
		if len(errItems) > 0 {
			fmt.Printf("%s[TASS]%s[WARNING] Не удалось обработать или отсутствовали критичные данные для %d из %d страниц:%s\n", ColorBlue, ColorYellow, len(errItems), totalLinks, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, LimitString(itemMessage, 250), ColorReset)
			}
		}
	} else if totalLinks > 0 {
		fmt.Printf("%s[TASS]%s[ERROR] Парсинг статей TASS.ru завершен, но не удалось собрать данные ни с одной из %d страниц.%s\n", ColorBlue, ColorRed, totalLinks, ColorReset)
		if len(errItems) > 0 {
			fmt.Printf("%s[TASS]%s[INFO] Список страниц с ошибками или без данных:%s\n", ColorBlue, ColorYellow, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, LimitString(itemMessage, 250), ColorReset)
			}
		}
	}
	return articles
}
