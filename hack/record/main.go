// Records a kbkb play session as docs/play.gif: connects to a headless
// Chrome (chromedp/headless-shell on :9222), plays the operating pair with
// scripted key presses, and screenshots the page into an animated GIF using
// only the Go standard library.
package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color/palette"
	"image/draw"
	"image/gif"
	"image/png"
	"os"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

// targetURL is where the headless Chrome finds the kbkb UI; from a Docker
// Desktop container the WSL/host port-forward is reachable via
// host.docker.internal.
func targetURL() string {
	if u := os.Getenv("KBKB_UI_URL"); u != "" {
		return u
	}
	return "http://host.docker.internal:8765"
}

func main() {
	allocCtx, cancelAlloc := chromedp.NewRemoteAllocator(context.Background(), "http://localhost:9222/")
	defer cancelAlloc()
	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	var title string
	if err := chromedp.Run(ctx,
		chromedp.EmulateViewport(1000, 640),
		chromedp.Navigate(targetURL()),
		chromedp.Sleep(2*time.Second),
		chromedp.Title(&title),
	); err != nil {
		panic(err)
	}
	if title != "kbkb" {
		panic(fmt.Sprintf("unexpected page title %q — is the UI reachable from the container?", title))
	}

	// scripted play: move, rotate (Z/X), soft drop, hard drop; a second quick
	// play once the next pair arrives
	go func() {
		type step struct {
			at  time.Duration
			key string
		}
		steps := []step{
			{0, kb.ArrowLeft},
			{600 * time.Millisecond, kb.ArrowLeft},
			{1300 * time.Millisecond, "z"},
			{2000 * time.Millisecond, kb.ArrowDown},
			{2400 * time.Millisecond, kb.ArrowDown},
			{3100 * time.Millisecond, "x"},
			{3800 * time.Millisecond, kb.ArrowDown},
			{4600 * time.Millisecond, kb.ArrowUp},
			{8500 * time.Millisecond, kb.ArrowRight},
			{9200 * time.Millisecond, "z"},
			{10000 * time.Millisecond, kb.ArrowUp},
		}
		start := time.Now()
		for _, s := range steps {
			time.Sleep(time.Until(start.Add(s.at)))
			_ = chromedp.Run(ctx, chromedp.KeyEvent(s.key))
		}
	}()

	const frames = 44
	const interval = 650 * time.Millisecond
	g := &gif.GIF{}
	for i := 0; i < frames; i++ {
		t0 := time.Now()
		var buf []byte
		if err := chromedp.Run(ctx, chromedp.CaptureScreenshot(&buf)); err != nil {
			panic(err)
		}
		img, err := png.Decode(bytes.NewReader(buf))
		if err != nil {
			panic(err)
		}
		// flat UI colors map cleanly onto the Plan9 palette without dithering
		pal := image.NewPaletted(img.Bounds(), palette.Plan9)
		draw.Draw(pal, img.Bounds(), img, image.Point{}, draw.Src)
		g.Image = append(g.Image, pal)
		g.Delay = append(g.Delay, int(interval/(10*time.Millisecond)))
		fmt.Printf("frame %d/%d\n", i+1, frames)
		if d := interval - time.Since(t0); d > 0 {
			time.Sleep(d)
		}
	}

	f, err := os.Create("play.gif")
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := gif.EncodeAll(f, g); err != nil {
		panic(err)
	}
	fmt.Println("wrote play.gif")
}
