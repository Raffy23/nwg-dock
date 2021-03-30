package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/allan-simon/go-singleinstance"
	"github.com/dlasky/gotk3-layershell/layershell"
	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

const version = "0.0.1"

var (
	appDirs                            []string
	configDirectory                    string
	pinnedFile                         string
	pinned                             []string
	oldTasks                           []task
	mainBox                            *gtk.Box
	src                                glib.SourceHandle
	refresh                            bool // we will use this to trigger rebuilding mainBox
	outerOrientation, innerOrientation gtk.Orientation
	widgetAnchor, menuAnchor           gdk.Gravity
)

// Flags
var cssFileName = flag.String("s", "style.css", "Styling: css file name")
var displayVersion = flag.Bool("v", false, "display Version information")
var autohide = flag.Bool("a", false, "Auto-hide: close window when left or a button clicked")
var numWS = flag.Int("w", 8, "number of Workspaces you use")
var position = flag.String("p", "bottom", "Position: bottom, top or left")
var exclusive = flag.Bool("x", false, "set eXclusive zone")
var imgSize = flag.Int("i", 48, "Icon size")

func buildMainBox(tasks []task, vbox *gtk.Box) {
	mainBox.Destroy()
	mainBox, _ = gtk.BoxNew(innerOrientation, 0)
	vbox.PackStart(mainBox, true, false, 0)

	var err error
	pinned, err = loadTextFile(pinnedFile)
	if err != nil {
		pinned = nil
	}

	var alreadyAdded []string
	for _, pin := range pinned {
		if !inTasks(tasks, pin) {
			button := pinnedButton(pin)
			mainBox.PackStart(button, false, false, 0)
		} else {
			instances := taskInstances(pin, tasks)
			task := instances[0]
			if len(instances) == 1 {
				button := taskButton(task, instances)
				mainBox.PackStart(button, false, false, 0)
			} else if !isIn(alreadyAdded, task.ID) {
				button := taskButton(task, instances)
				mainBox.PackStart(button, false, false, 0)
				alreadyAdded = append(alreadyAdded, task.ID)
				taskMenu(task.ID, instances)
			} else {
				continue
			}
		}
	}

	alreadyAdded = nil
	for _, task := range tasks {
		// nwggrid is a companion app w/ the special button
		if !inPinned(task.ID) && task.ID != "nwggrid" {
			instances := taskInstances(task.ID, tasks)
			if len(instances) == 1 {
				button := taskButton(task, instances)
				mainBox.PackStart(button, false, false, 0)
			} else if !isIn(alreadyAdded, task.ID) {
				button := taskButton(task, instances)
				mainBox.PackStart(button, false, false, 0)
				alreadyAdded = append(alreadyAdded, task.ID)
				taskMenu(task.ID, instances)
			} else {
				continue
			}
		}
	}

	button, _ := gtk.ButtonNew()
	image, err := createImage("nwggrid", *imgSize)
	if err == nil {
		button.SetImage(image)
		button.SetImagePosition(gtk.POS_TOP)
		button.SetAlwaysShowImage(true)
		button.SetLabel("")

		button.Connect("clicked", func() {
			launch("nwggrid -p")
		})
		button.Connect("enter-notify-event", cancelClose)
	}
	mainBox.PackStart(button, false, false, 0)
	mainBox.ShowAll()
}

func main() {
	flag.Parse()

	if *displayVersion {
		fmt.Printf("nwg-dock version %s\n", version)
		os.Exit(0)
	}

	// Gentle SIGTERM handler thanks to reiki4040 https://gist.github.com/reiki4040/be3705f307d3cd136e85
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGTERM)
	go func() {
		for {
			s := <-signalChan
			if s == syscall.SIGTERM {
				fmt.Println("SIGTERM received, bye bye!")
				gtk.MainQuit()
			}
		}
	}()

	// We don't want multiple instances. Kill the running instance and exit.
	lockFilePath := fmt.Sprintf("%s/nwg-dock.lock", tempDir())
	lockFile, err := singleinstance.CreateLockFile(lockFilePath)
	if err != nil {
		pid, err := readTextFile(lockFilePath)
		if err == nil {
			i, err := strconv.Atoi(pid)
			if err == nil {
				fmt.Println("Running instance found, sending SIGTERM and exiting...")
				syscall.Kill(i, syscall.SIGTERM)
			}
		}
		os.Exit(0)
	}
	defer lockFile.Close()

	configDirectory = configDir()
	// if doesn't exist:
	createDir(configDirectory)

	cacheDirectory := cacheDir()
	if cacheDirectory == "" {
		log.Panic("Couldn't determine cache directory location")
	}
	pinnedFile = filepath.Join(cacheDirectory, "nwg-dock-pinned")
	cssFile := filepath.Join(configDirectory, *cssFileName)
	appDirs = getAppDirs()

	gtk.Init(nil)

	cssProvider, _ := gtk.CssProviderNew()

	err = cssProvider.LoadFromPath(cssFile)
	if err != nil {
		fmt.Printf("%s file not found, using GTK styling\n", cssFile)
	} else {
		fmt.Printf("Using style: %s\n", cssFile)
		screen, _ := gdk.ScreenGetDefault()
		gtk.AddProviderForScreen(screen, cssProvider, gtk.STYLE_PROVIDER_PRIORITY_USER)
	}

	win, err := gtk.WindowNew(gtk.WINDOW_TOPLEVEL)
	if err != nil {
		log.Fatal("Unable to create window:", err)
	}
	layershell.InitForWindow(win)

	if *exclusive {
		layershell.AutoExclusiveZoneEnable(win)
	}

	if *position == "bottom" || *position == "top" {
		if *position == "bottom" {
			layershell.SetAnchor(win, layershell.LAYER_SHELL_EDGE_BOTTOM, true)

			widgetAnchor = gdk.GDK_GRAVITY_NORTH
			menuAnchor = gdk.GDK_GRAVITY_SOUTH
		} else {
			layershell.SetAnchor(win, layershell.LAYER_SHELL_EDGE_TOP, true)

			widgetAnchor = gdk.GDK_GRAVITY_SOUTH
			menuAnchor = gdk.GDK_GRAVITY_NORTH
		}

		outerOrientation = gtk.ORIENTATION_VERTICAL
		innerOrientation = gtk.ORIENTATION_HORIZONTAL

		layershell.SetAnchor(win, layershell.LAYER_SHELL_EDGE_LEFT, *exclusive)
		layershell.SetAnchor(win, layershell.LAYER_SHELL_EDGE_RIGHT, *exclusive)
	}

	if *position == "left" {
		layershell.SetAnchor(win, layershell.LAYER_SHELL_EDGE_LEFT, true)

		layershell.SetAnchor(win, layershell.LAYER_SHELL_EDGE_TOP, *exclusive)
		layershell.SetAnchor(win, layershell.LAYER_SHELL_EDGE_BOTTOM, *exclusive)

		outerOrientation = gtk.ORIENTATION_HORIZONTAL
		innerOrientation = gtk.ORIENTATION_VERTICAL

		widgetAnchor = gdk.GDK_GRAVITY_EAST
		menuAnchor = gdk.GDK_GRAVITY_WEST
	}

	layershell.SetLayer(win, layershell.LAYER_SHELL_LAYER_TOP)
	layershell.SetMargin(win, layershell.LAYER_SHELL_EDGE_TOP, 0)
	layershell.SetMargin(win, layershell.LAYER_SHELL_EDGE_LEFT, 0)
	layershell.SetMargin(win, layershell.LAYER_SHELL_EDGE_RIGHT, 0)
	layershell.SetMargin(win, layershell.LAYER_SHELL_EDGE_BOTTOM, 0)

	win.Connect("destroy", func() {
		gtk.MainQuit()
	})

	// Close the window on leave, but not immediately, to avoid accidental closes

	win.Connect("leave-notify-event", func() {
		if *autohide {
			src, err = glib.TimeoutAdd(uint(1000), func() bool {
				gtk.MainQuit()
				return false
			})
		}
	})

	win.Connect("enter-notify-event", func() {
		cancelClose()
	})

	outerBox, _ := gtk.BoxNew(outerOrientation, 0)
	outerBox.SetProperty("name", "box")
	win.Add(outerBox)

	alignmentBox, _ := gtk.BoxNew(innerOrientation, 0)
	outerBox.PackStart(alignmentBox, true, true, 0)

	mainBox, _ = gtk.BoxNew(innerOrientation, 0)
	alignmentBox.PackStart(mainBox, true, false, 0)

	tasks, err := listTasks()
	if err != nil {
		log.Fatal("Couldn't list tasks:", err)
	}
	oldTasks = tasks

	buildMainBox(tasks, alignmentBox)

	glib.TimeoutAdd(uint(150), func() bool {
		currentTasks, _ := listTasks()
		if len(currentTasks) != len(oldTasks) || refresh {
			fmt.Println("refreshing...")
			buildMainBox(currentTasks, outerBox)
			oldTasks = currentTasks
			refresh = false
		}
		return true
	})

	win.ShowAll()
	gtk.Main()
}
