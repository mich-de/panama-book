package main

import (
	"runtime"

	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// pickFolder apre «Cerca cartella» di Windows (Shell.Application.BrowseForFolder) e restituisce la cartella scelta,
// oppure "" se l'operatore annulla. start non si usa: la finestra parte dal Desktop (il parametro root la limiterebbe).
func pickFolder(title, start string) (string, error) {
	// STA: tutte le chiamate COM restano sullo stesso thread OS
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err == nil {
		defer ole.CoUninitialize()
	}
	unk, err := oleutil.CreateObject("Shell.Application")
	if err != nil {
		return "", err
	}
	defer unk.Release()
	sh, err := unk.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return "", err
	}
	defer sh.Release()
	// 0x1 solo cartelle del file system | 0x10 casella di testo | 0x40 finestra moderna con «Crea nuova cartella»
	res, err := oleutil.CallMethod(sh, "BrowseForFolder", 0, title, 0x51)
	if err != nil {
		return "", err
	}
	folder := res.ToIDispatch()
	if folder == nil { // annullato
		return "", nil
	}
	defer folder.Release()
	self, err := oleutil.GetProperty(folder, "Self")
	if err != nil {
		return "", err
	}
	item := self.ToIDispatch()
	defer item.Release()
	path, err := oleutil.GetProperty(item, "Path")
	if err != nil {
		return "", err
	}
	return path.ToString(), nil
}
