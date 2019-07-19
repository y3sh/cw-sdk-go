package main

import (
	"github.com/rivo/tview"
)

type MessageViewParams struct {
	MessageID       string
	Title           string
	Message         string
	Buttons         []string
	OnButtonPressed func(label string, idx int)

	// Width and Height are 40 and 10 by default
	Width, Height int
}

type MessageView struct {
	params   MessageViewParams
	mainView *MainView

	msgboxFlex *tview.Flex
	frame      *tview.Frame

	textView *tview.TextView
	buttons  []*tview.Button
}

func NewMessageView(
	mainView *MainView, params *MessageViewParams,
) *MessageView {
	msgv := &MessageView{
		params:   *params,
		mainView: mainView,
	}

	if msgv.params.Width == 0 {
		msgv.params.Width = 40
	}

	if msgv.params.Height == 0 {
		msgv.params.Height = 10
	}

	msgv.msgboxFlex = tview.NewFlex().SetDirection(tview.FlexRow)

	msgv.textView = tview.NewTextView()
	msgv.textView.SetText(params.Message)
	msgv.textView.SetTextAlign(tview.AlignCenter)

	msgv.msgboxFlex.AddItem(msgv.textView, 0, 1, len(params.Buttons) == 0)

	for i, b := range params.Buttons {
		btnLabel := b
		btnIdx := i
		btn := tview.NewButton(btnLabel).SetSelectedFunc(func() {
			params.OnButtonPressed(btnLabel, btnIdx)
		})

		// TODO: use horizontal flex for buttons
		msgv.msgboxFlex.AddItem(btn, 1, 0, i == 0)
	}

	msgv.frame = tview.NewFrame(msgv.msgboxFlex).SetBorders(0, 0, 0, 0, 0, 0)
	msgv.frame.SetBorder(true).SetBorderPadding(1, 1, 1, 1)
	msgv.frame.SetTitle(params.Title)

	return msgv
}

func (msgv *MessageView) Show() {
	msgv.mainView.showModal(
		pageNameMessage+msgv.params.MessageID, msgv.frame,
		msgv.params.Width,
		msgv.params.Height,
	)
}

func (msgv *MessageView) Hide() {
	msgv.mainView.hideModal(pageNameMessage + msgv.params.MessageID)
}
