package vultrconsole

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

var ErrChallenge = errors.New("vultr console challenge required")
var limitRE = regexp.MustCompile(`(?i)maximum\s+instances?\s*[:\n]?\s*(\d+)`)

type BrowserObserver struct {
	ChromePath string
	Timeout    time.Duration
}

func (b BrowserObserver) Observe(ctx context.Context, email, password, proxyURL string) (int, error) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(b.ChromePath),
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
	)
	if proxyURL != "" {
		opts = append(opts, chromedp.ProxyServer(proxyURL))
	}
	allocCtx, cancel := chromedp.NewExecAllocator(ctx, opts...)
	defer cancel()
	bctx, cancelBrowser := chromedp.NewContext(allocCtx)
	defer cancelBrowser()
	timeout := b.Timeout
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	bctx, cancelTimeout := context.WithTimeout(bctx, timeout)
	defer cancelTimeout()

	var body, currentURL, title string
	if err := chromedp.Run(bctx, chromedp.Navigate("https://my.vultr.com/"), chromedp.WaitReady("body", chromedp.ByQuery), chromedp.Location(&currentURL), chromedp.Title(&title)); err != nil {
		return 0, fmt.Errorf("navigate: %w", err)
	}
	if strings.Contains(strings.ToLower(title), "just a moment") || strings.Contains(strings.ToLower(title), "security check") {
		return 0, ErrChallenge
	}
	loginCtx, cancelLogin := context.WithTimeout(bctx, 12*time.Second)
	defer cancelLogin()
	if err := chromedp.Run(loginCtx, chromedp.WaitVisible(`input[type=email],input[name*=email i],input[autocomplete=username]`, chromedp.ByQuery)); err != nil {
		return 0, fmt.Errorf("login_form url=%s title=%q: %w", currentURL, title, err)
	}
	if err := chromedp.Run(bctx, chromedp.SetValue(`input[type=email],input[name*=email i],input[autocomplete=username]`, email, chromedp.ByQuery), chromedp.SetValue(`input[type=password]`, password, chromedp.ByQuery), chromedp.Submit(`input[type=password]`, chromedp.ByQuery), chromedp.Sleep(3*time.Second), chromedp.Navigate("https://my.vultr.com/settings/#settingslimits"), chromedp.Sleep(2*time.Second), chromedp.Location(&currentURL), chromedp.Title(&title), chromedp.Text("body", &body, chromedp.ByQuery)); err != nil {
		return 0, fmt.Errorf("limits_page url=%s title=%q: %w", currentURL, title, err)
	}
	lower := strings.ToLower(body)
	if strings.Contains(lower, "two-factor") ||
		strings.Contains(lower, "verification code") ||
		strings.Contains(lower, "captcha") ||
		strings.Contains(lower, "security challenge") {
		return 0, ErrChallenge
	}
	m := limitRE.FindStringSubmatch(body)
	if len(m) != 2 {
		return 0, errors.New("maximum instances not found")
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n < 1 {
		return 0, errors.New("invalid maximum instances")
	}
	return n, nil
}
