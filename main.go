package main

import (
	"bufio"
	"fmt"
	"os"
	"parsing_media/parsers"
	. "parsing_media/utils"
	"sync"
	"time"
)

func main() {
	for true {
		var num string
		fmt.Printf("%sMENU BAR%s\n! - Exit\n0 - Run all\n1 - RIA\n2 - Gazeta\n3 - Lenta\n4 - Vesti\n5 - Kommersant\n6 - MK\n7 - Fontanka\n8 - Smotrim\n9 - Banki\n10 - DumaTV\n11 - RBC\n12 - Izvestiya\n13 - Interfax\n14 - RG\n15 - KP\nOption: ", ColorYellow, ColorReset)
		fmt.Scan(&num)

		switch num {
		case "!":
			return

		case "0":
			interruptChan := make(chan struct{})
			var interruptOnce sync.Once

			go func() {
				reader := bufio.NewReader(os.Stdin)
				_, _ = reader.ReadString('\n')
				interruptOnce.Do(func() {
					close(interruptChan)
				})
			}()

			keepRunning := true
			for keepRunning {
				var wg sync.WaitGroup

				parserJobs := []struct {
					name string
					fn   func()
				}{
					{"RIA", parsers.RiaMain},
					{"Gazeta", parsers.GazetaMain},
					{"Lenta", parsers.LentaMain},
					{"Vesti", parsers.VestiMain},
					{"Kommersant", parsers.KommersMain},
					{"MK", parsers.MKMain},
					{"Fontanka", parsers.FontankaMain},
					{"Smotrim", parsers.SmotrimMain},
					{"Banki", parsers.BankiMain},
					{"DumaTV", parsers.DumaTVMain},
					{"RBC", parsers.RbcMain},
					{"Izvestiya", parsers.IzMain},
					{"Interfax", parsers.InterfaxMain},
					{"RG", parsers.RGMain},
					{"KP", parsers.KPMain},
				}

				fmt.Printf("\n%s[INFO] Запуск всех парсеров%s\n\n", ColorBlue, ColorReset)

				for _, job := range parserJobs {
					wg.Add(1)
					go func(parserName string, parserFunc func()) {
						defer wg.Done()
						parserFunc()
					}(job.name, job.fn)
				}

				wg.Wait()

				fmt.Printf("%s[INFO] Все парсеры (%d) завершили свою работу.%s\n", ColorBlue, len(parserJobs), ColorReset)

				select {
				case <-interruptChan:
					fmt.Printf("%s[INFO] Обнаружено нажатие Enter. Остановка цикла...%s\n", ColorBlue, ColorReset)
					keepRunning = false
					continue
				default:
				}

				if !keepRunning {
					break
				}

				fmt.Printf("%s[INFO] Нажмите Enter для остановки.%s\n", ColorBlue, ColorReset)

				totalWaitDuration := 3 * time.Minute
				timer := time.NewTimer(totalWaitDuration)

				stopCountdownChan := make(chan struct{})
				var wgCountdown sync.WaitGroup
				wgCountdown.Add(1)

				go func() {
					defer wgCountdown.Done()
					// Update ~30 times per second (1000ms / 30fps ≈ 33ms)
					// For ~60fps, use 16*time.Millisecond or 17*time.Millisecond
					ticker := time.NewTicker(33 * time.Millisecond)
					defer ticker.Stop()

					deadline := time.Now().Add(totalWaitDuration)

					for {
						select {
						case <-stopCountdownChan:
							return
						case <-ticker.C:
							currentRemaining := time.Until(deadline)
							if currentRemaining <= 0 {
								fmt.Printf("\r%s[INFO] До повторного запуска (0m 0.000s)%s", ColorBlue, ColorReset)
								return
							}
							minutes := int(currentRemaining.Minutes())
							seconds := currentRemaining.Seconds() - float64(minutes*60)
							fmt.Printf("\r%s[INFO] До повторного запуска (%dm %.3fs)%s", ColorBlue, minutes, seconds, ColorReset)
						}
					}
				}()

				select {
				case <-interruptChan:
					interruptOnce.Do(func() { close(interruptChan) })

					close(stopCountdownChan)
					wgCountdown.Wait()

					fmt.Printf("\n%s[INFO] Обнаружено нажатие Enter. Остановка цикла...%s\n", ColorBlue, ColorReset)
					keepRunning = false
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
				case <-timer.C:
					close(stopCountdownChan)
					wgCountdown.Wait()

					fmt.Printf("\r%s[INFO] До повторного запуска (0m 0.000s)%s\n", ColorBlue, ColorReset)
					fmt.Printf("%s[INFO] Таймаут истек. Перезапуск парсеров...%s\n", ColorBlue, ColorReset)
				}
			}
			interruptOnce.Do(func() { close(interruptChan) })
			fmt.Printf("\n%s[INFO] Цикл парсинга завершен.%s\n", ColorBlue, ColorReset)
		case "1":
			parsers.RiaMain()
		case "2":
			parsers.GazetaMain()
		case "3":
			parsers.LentaMain()
		case "4":
			parsers.VestiMain()
		case "5":
			parsers.KommersMain()
		case "6":
			parsers.MKMain()
		case "7":
			parsers.FontankaMain()
		case "8":
			parsers.SmotrimMain()
		case "9":
			parsers.BankiMain()
		case "10":
			parsers.DumaTVMain()
		case "11":
			parsers.RbcMain()
		case "12":
			parsers.IzMain()
		case "13":
			parsers.InterfaxMain()
		case "14":
			parsers.RGMain()
		case "15":
			parsers.KPMain()
		}
	}
}
