package menu

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/studentkittens/eulenfunk/display"
	"github.com/studentkittens/eulenfunk/util"
)

type Action func() error

type Entry struct {
	Text   string
	Action Action
}

type Menu struct {
	Name    string
	Entries []*Entry
	Cursor  int

	lw *display.LineWriter
}

func NewMenu(name string, lw *display.LineWriter) (*Menu, error) {
	return &Menu{
		Name: name,
		lw:   lw,
	}, nil
}

func (mn *Menu) AddEntry(entry *Entry) {
	mn.Entries = append(mn.Entries, entry)
}

func (mn *Menu) ActiveName() string {
	if len(mn.Entries) == 0 {
		return ""
	}

	return mn.Entries[mn.Cursor].Text
}

func (mn *Menu) Scroll(move int) {
	mn.Cursor += move
	if mn.Cursor < 0 {
		mn.Cursor = 0
	}

	if mn.Cursor >= len(mn.Entries) {
		mn.Cursor = len(mn.Entries) - 1
	}
}

func (mn *Menu) Display() error {
	for pos, entry := range mn.Entries {
		line := entry.Text

		if pos == mn.Cursor {
			line = "> " + line
		} else {
			line = "  " + line
		}

		if _, err := mn.lw.Formatf("line %s %d %s", mn.Name, pos, line); err != nil {
			return err
		}
	}

	return nil
}

func (mn *Menu) Click() error {
	if len(mn.Entries) == 0 {
		return nil
	}

	entry := mn.Entries[mn.Cursor]
	if entry.Action != nil {
		return nil
	}

	return entry.Action()
}

////////////////////////

const (
	// No movement (initial)
	DirectionNone = iota
	DirectionRight
	DirectionLeft
)

type MenuManager struct {
	sync.Mutex

	Active       *Menu
	Menus        map[string]*Menu
	TimedActions map[time.Duration]Action

	lw                   *display.LineWriter
	rotateActions        []Action
	currValue, lastValue int
	rotary               *util.Rotary
}

func NewMenuManager(lw *display.LineWriter) (*MenuManager, error) {
	rty, err := util.NewRotary()
	if err != nil {
		return nil, err
	}

	mgr := &MenuManager{
		Menus:  make(map[string]*Menu),
		lw:     lw,
		rotary: rty,
	}

	go func() {
		for state := range rty.Button {
			if mgr.Active == nil {
				continue
			}

			if state {
				// We don't do anything yet...
				fmt.Println("Button pressed")
				continue
			}

			fmt.Println("Button released")

			mgr.Lock()
			if err := mgr.Active.Click(); err != nil {
				active := mgr.Active.ActiveName()
				log.Printf("Action for menu entry `%s` failed: %v", active, err)
			}
			mgr.Unlock()
		}
	}()

	go func() {
		for duration := range rty.Pressed {
			mgr.Lock()

			// Find the action with smallest non-negative diff:
			var diff time.Duration
			var action Action

			for after, timedAction := range mgr.TimedActions {
				newDiff := duration - after
				if after <= duration && (action == nil || newDiff < diff) {
					diff = duration - after
					action = timedAction
				}
			}

			mgr.Unlock()

			if action != nil {
				action()
			}
		}
	}()

	go func() {
		for value := range rty.Value {
			fmt.Printf("Value: %d\n", value)

			mgr.Lock()
			mgr.Active.Scroll(value)
			mgr.lastValue = mgr.currValue
			mgr.currValue = value
			mgr.Unlock()

			if _, err := lw.Formatf("move menu %d", value); err != nil {
				log.Printf("move failed: %v", err)
			}

			for idx, action := range mgr.rotateActions {
				if err := action(); err != nil {
					log.Printf("Rotate action %d failed: %v", idx, err)
				}
			}

			mgr.Active.Display()
		}
	}()

	return mgr, nil
}

func (mgr *MenuManager) Direction() int {
	mgr.Lock()
	defer mgr.Unlock()

	switch {
	case mgr.lastValue < mgr.currValue:
		return DirectionRight
	case mgr.lastValue > mgr.currValue:
		return DirectionLeft
	default:
		return DirectionNone
	}
}

func (mgr *MenuManager) Value() int {
	mgr.Lock()
	defer mgr.Unlock()

	return mgr.currValue
}

func (mgr *MenuManager) RotateAction(a Action) {
	mgr.Lock()
	defer mgr.Unlock()

	mgr.rotateActions = append(mgr.rotateActions, a)
}

func (mgr *MenuManager) SwitchTo(name string) error {
	if _, err := mgr.lw.Formatf("switch %s", name); err != nil {
		log.Printf("switch failed: %v", err)
		return err
	}

	mgr.Lock()
	defer mgr.Unlock()

	if menu, ok := mgr.Menus[name]; ok {
		mgr.Active = menu
		mgr.Active.Display()
	}

	return nil
}

func (mgr *MenuManager) AddTimedAction(after time.Duration, action Action) {
	mgr.Lock()
	defer mgr.Unlock()

	mgr.TimedActions[after] = action
}

func (mgr *MenuManager) AddMenu(name string, entries []*Entry) error {
	mgr.Lock()
	defer mgr.Unlock()

	menu, err := NewMenu(name, mgr.lw)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		menu.AddEntry(entry)
	}

	if mgr.Active == nil {
		mgr.Active = menu
		mgr.Active.Display()
	}

	mgr.Menus[name] = menu
	return nil
}

func (mgr *MenuManager) Close() error {
	return mgr.rotary.Close()
}

//////////////////////////////////////

func switcher(lw *display.LineWriter, name string) func() error {
	return func() error {
		_, err := lw.Formatf("switch %s", name)
		return err
	}
}

func Run() error {
	cfg := &display.Config{
		Host: "localhost",
		Port: 7778,
	}

	lw, err := display.Connect(cfg)
	if err != nil {
		return err
	}

	defer lw.Close()

	mgr, err := NewMenuManager(lw)
	if err != nil {
		return err
	}

	defer mgr.Close()

	// Start clock and sysinfo screen:
	killClock, killSysinfo := make(chan bool), make(chan bool)
	go RunClock(lw, 20, killClock) // TODO: get width?
	go RunSysinfo(lw, 20, killSysinfo)

	mainMenu := []*Entry{
		{
			"Exit", switcher(lw, "mpd"),
		}, {
			"Playlists", switcher(lw, "playlists"),
		}, {
			"Toggle PartyMode", nil, // TODO
		}, {
			"System info", switcher(lw, "sysinfo"),
		}, {
			"Clock", switcher(lw, "clock"),
		}, {
			"Stop playback", nil, // TODO
		}, {
			"Power", switcher(lw, "menu-power"),
		},
	}

	powerMenu := []*Entry{
		{
			"Poweroff", nil, // TODO
		}, {
			"Reboot", nil, // TODO
		},
	}

	easterEggMenu := []*Entry{
		{
			"Schuhu?", nil,
		},
	}

	if err := mgr.AddMenu("menu-main", mainMenu); err != nil {
		return err
	}

	if err := mgr.AddMenu("menu-power", powerMenu); err != nil {
		return err
	}

	if err := mgr.AddMenu("menu-easteregg", easterEggMenu); err != nil {
		return err
	}

	mgr.AddTimedAction(10*time.Millisecond, func() error {
		log.Printf("TODO: Toggle playback")
		return nil
	})

	mgr.AddTimedAction(500*time.Millisecond, func() error {
		return mgr.SwitchTo("menu-main")
	})

	mgr.AddTimedAction(2*time.Second, func() error {
		return mgr.SwitchTo("menu-main")
	})

	mgr.AddTimedAction(10*time.Second, func() error {
		return mgr.SwitchTo("menu-easteregg")
	})

	mgr.RotateAction(func() error {
		switch mgr.Direction() {
		case DirectionRight:
			log.Printf("Play next")
		case DirectionLeft:
			log.Printf("Play prev")
		}

		return nil
	})

	log.Printf("Press CTRL-C to shut down")
	ctrlCh := make(chan os.Signal, 1)
	signal.Notify(ctrlCh, os.Interrupt)
	<-ctrlCh

	killClock <- true
	killSysinfo <- true

	return nil
}
