package helpers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"math/rand"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	tgmd2html "github.com/PaulSonOfLars/gotg_md2html"
	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	log "github.com/sirupsen/logrus"

	"github.com/divideprojects/Alita_Robot/alita/config"
	"github.com/divideprojects/Alita_Robot/alita/db"
	"github.com/divideprojects/Alita_Robot/alita/i18n"
	"github.com/divideprojects/Alita_Robot/alita/utils/chat_status"
	"github.com/divideprojects/Alita_Robot/alita/utils/extraction"
	"github.com/divideprojects/Alita_Robot/alita/utils/string_handling"
)

// NOTE: small helper functions
// constants
const (
	Markdown             = "Markdown"
	HTML                 = "HTML"
	None                 = "None"
	MaxMessageLength int = 4096
)

// Shtml returns a SendMessageOpts pointer configured for HTML parse mode.
//
// Disables link previews and allows sending without reply by default.
func Shtml() *gotgbot.SendMessageOpts {
	return &gotgbot.SendMessageOpts{
		ParseMode: HTML,
		LinkPreviewOptions: &gotgbot.LinkPreviewOptions{
			IsDisabled: true,
		},
		ReplyParameters: &gotgbot.ReplyParameters{
			AllowSendingWithoutReply: true,
		},
	}
}

// Smarkdown returns a SendMessageOpts pointer configured for Markdown parse mode.
//
// Disables link previews and allows sending without reply by default.
func Smarkdown() *gotgbot.SendMessageOpts {
	return &gotgbot.SendMessageOpts{
		ParseMode: Markdown,
		LinkPreviewOptions: &gotgbot.LinkPreviewOptions{
			IsDisabled: true,
		},
		ReplyParameters: &gotgbot.ReplyParameters{
			AllowSendingWithoutReply: true,
		},
	}
}

// SplitMessage splits a message into multiple strings if it exceeds MaxMessageLength.
//
// Returns a slice of message parts, each within the allowed length.
func SplitMessage(msg string) []string {
	if len(msg) > MaxMessageLength {
		tmp := make([]string, 1)
		tmp[0] = msg
		return tmp
	} else {
		lines := strings.Split(msg, "\n")
		smallMsg := ""
		result := make([]string, 0)
		for _, line := range lines {
			if len(smallMsg)+len(line) < MaxMessageLength {
				smallMsg += line + "\n"
			} else {
				result = append(result, smallMsg)
				smallMsg = line + "\n"
			}
		}
		result = append(result, smallMsg)
		return result
	}
}

// MentionHtml returns an HTML-formatted mention link for a user.
//
// Uses the user's ID and display name.
func MentionHtml(userId int64, name string) string {
	return MentionUrl(fmt.Sprintf("tg://user?id=%d", userId), name)
}

// MentionUrl returns an HTML-formatted link with the given URL and display name.
//
// Escapes the name for HTML safety.
func MentionUrl(url, name string) string {
	return fmt.Sprintf("<a href=\"%s\">%s</a>", url, html.EscapeString(name))
}

/*
GetFullName returns the full name by combining first and last names.

If the last name is empty, returns only the first name.
*/
func GetFullName(FirstName, LastName string) string {
	var name string
	if LastName != "" {
		name = FirstName + " " + LastName
	} else {
		name = FirstName
	}
	return name
}

/*
InitButtons initializes the inline keyboard buttons for the connection menu.

Admins see both admin and user command buttons; regular users see only user commands.
*/
func InitButtons(b *gotgbot.Bot, chatId, userId int64) gotgbot.InlineKeyboardMarkup {
	var connButtons [][]gotgbot.InlineKeyboardButton
	if chat_status.IsUserAdmin(b, chatId, userId) {
		connButtons = [][]gotgbot.InlineKeyboardButton{
			{
				{
					Text:         "Admin commands",
					CallbackData: "connbtns.Admin",
				},
			},
			{
				{
					Text:         "User commands",
					CallbackData: "connbtns.User",
				},
			},
		}
	} else {
		connButtons = [][]gotgbot.InlineKeyboardButton{
			{
				{
					Text:         "User commands",
					CallbackData: "connbtns.User",
				},
			},
		}
	}
	connKeyboard := gotgbot.InlineKeyboardMarkup{InlineKeyboard: connButtons}
	return connKeyboard
}

// GetMessageLinkFromMessageId constructs a t.me message link from chat and message IDs.
//
// Handles both username-based and ID-based chats, properly formatting the link
// based on whether the chat has a username or uses a numeric ID.
func GetMessageLinkFromMessageId(chat *gotgbot.Chat, messageId int64) (messageLink string) {
	messageLink = "https://t.me/"
	chatIdStr := fmt.Sprint(chat.Id)
	if chat.Username == "" {
		var linkId string
		if strings.HasPrefix(chatIdStr, "-100") {
			linkId = strings.ReplaceAll(chatIdStr, "-100", "")
		} else if strings.HasPrefix(chatIdStr, "-") && !strings.HasPrefix(chatIdStr, "-100") {
			// this is for non-supergroups
			linkId = strings.ReplaceAll(chatIdStr, "-", "")
		}
		messageLink += fmt.Sprintf("c/%s/%d", linkId, messageId)
	} else {
		messageLink += fmt.Sprintf("%s/%d", chat.Username, messageId)
	}
	return
}

// NOTE: connection helper functions

/*
IsUserConnected checks if a user is connected to a chat, considering admin requirements.

If in a private chat, checks the user's connection; otherwise, uses the current chat.
Returns the connected chat or nil if requirements are not met.
*/
func IsUserConnected(b *gotgbot.Bot, ctx *ext.Context, chatAdmin, botAdmin bool) (chat *gotgbot.Chat) {
	msg := ctx.EffectiveMessage
	user := ctx.EffectiveUser
	tr := i18n.I18n{LangCode: db.GetLanguage(ctx)}

	if ctx.Message.Chat.Type == "private" {
		conn := db.Connection(user.Id)
		if conn.Connected && conn.ChatId != 0 {
			chatFullInfo, err := b.GetChat(conn.ChatId, nil)
			if err != nil {
				log.Error(err)
				return nil
			}
			_chat := chatFullInfo.ToChat() // need to convert to Chat type
			chat = &_chat
		} else {
			_, err := msg.Reply(b,
				tr.GetString("strings.connections.is_user_connected.need_group"),
				&gotgbot.SendMessageOpts{
					ReplyParameters: &gotgbot.ReplyParameters{
						MessageId:                msg.MessageId,
						AllowSendingWithoutReply: true,
					},
				},
			)
			if err != nil {
				log.Error(err)
				return nil
			}

			return nil
		}
	} else {
		chat = ctx.EffectiveChat
	}
	if botAdmin {
		if !chat_status.IsUserAdmin(b, chat.Id, b.Id) {
			_, err := msg.Reply(b, tr.GetString("strings.connections.is_user_connected.bot_not_admin"), Shtml())
			if err != nil {
				log.Error(err)
				return nil
			}

			return nil
		}
	}
	if chatAdmin {
		if !chat_status.IsUserAdmin(b, chat.Id, user.Id) {
			_, err := msg.Reply(b, tr.GetString("strings.connections.is_user_connected.user_not_admin"), Shtml())
			if err != nil {
				log.Error(err)
				return nil
			}

			return nil
		}
	}
	ctx.EffectiveChat = chat
	return chat
}

// NOTE: keyboard helper functions

// BuildKeyboard constructs a 2D slice of InlineKeyboardButton from a list of db.Button.
//
// Handles button placement on the same line or new lines as specified.
func BuildKeyboard(buttons []db.Button) [][]gotgbot.InlineKeyboardButton {
	keyb := make([][]gotgbot.InlineKeyboardButton, 0)
	for _, btn := range buttons {
		if btn.SameLine && len(keyb) > 0 {
			keyb[len(keyb)-1] = append(keyb[len(keyb)-1], gotgbot.InlineKeyboardButton{Text: btn.Name, Url: btn.Url})
		} else {
			k := make([]gotgbot.InlineKeyboardButton, 1)
			k[0] = gotgbot.InlineKeyboardButton{Text: btn.Name, Url: btn.Url}
			keyb = append(keyb, k)
		}
	}
	return keyb
}

/*
ConvertButtonV2ToDbButton converts a slice of tgmd2html.ButtonV2 to a slice of db.Button.

Preserves button names, URLs, and line placement.
*/
func ConvertButtonV2ToDbButton(buttons []tgmd2html.ButtonV2) (btns []db.Button) {
	btns = make([]db.Button, len(buttons))
	for i, btn := range buttons {
		btns[i] = db.Button{
			Name:     btn.Name,
			Url:      btn.Content,
			SameLine: btn.SameLine,
		}
	}
	return
}

/*
RevertButtons converts a slice of db.Button to a formatted string representation.

Used for serializing button layouts as text.
*/
func RevertButtons(buttons []db.Button) string {
	res := ""
	for _, btn := range buttons {
		if btn.SameLine {
			res += fmt.Sprintf("\n[%s](buttonurl://%s)", btn.Name, btn.Url)
		} else {
			res += fmt.Sprintf("\n[%s](buttonurl://%s:same)", btn.Name, btn.Url)
		}
	}
	return res
}

/*
InlineKeyboardMarkupToTgmd2htmlButtonV2 converts InlineKeyboardMarkup to a slice of tgmd2html.ButtonV2.

Extracts button text, URLs, and line placement for further processing.
*/
func InlineKeyboardMarkupToTgmd2htmlButtonV2(replyMarkup *gotgbot.InlineKeyboardMarkup) (btns []tgmd2html.ButtonV2) {
	btns = make([]tgmd2html.ButtonV2, 0)
	for _, inlineKeyboard := range replyMarkup.InlineKeyboard {
		if len(inlineKeyboard) > 1 {
			for i, button := range inlineKeyboard {
				// if any button has anything other than url, it's not a valid button
				// skip options such as CallbackData, CallbackUrl, etc.
				if button.Url == "" {
					continue
				}

				sameline := true
				if i == 0 {
					sameline = false
				}
				btns = append(
					btns,
					tgmd2html.ButtonV2{
						Name:     button.Text,
						Content:  button.Url,
						SameLine: sameline,
					},
				)
			}
		} else {
			btns = append(btns,
				tgmd2html.ButtonV2{
					Name:     inlineKeyboard[0].Text,
					Content:  inlineKeyboard[0].Url,
					SameLine: false,
				},
			)
		}
	}
	return
}

/*
ChunkKeyboardSlices splits a slice of InlineKeyboardButton into chunks of the specified size.

Used for organizing buttons into rows for keyboards.
*/
func ChunkKeyboardSlices(slice []gotgbot.InlineKeyboardButton, chunkSize int) (chunks [][]gotgbot.InlineKeyboardButton) {
	for len(slice) > 0 {
		if len(slice) < chunkSize {
			chunkSize = len(slice)
		}

		chunks = append(chunks, slice[0:chunkSize])
		slice = slice[chunkSize:]

	}
	return chunks
}

// NOTE: language helper functions

/*
MakeLanguageKeyboard creates an inline keyboard with buttons for all supported languages.

Each button allows the user to change the bot's language setting.
*/
func MakeLanguageKeyboard() [][]gotgbot.InlineKeyboardButton {
	var kb []gotgbot.InlineKeyboardButton

	for _, langCode := range config.ValidLangCodes {
		properLang := GetLangFormat(langCode)
		if properLang == "" || properLang == " " {
			continue
		}

		kb = append(
			kb,
			gotgbot.InlineKeyboardButton{
				Text:         properLang,
				CallbackData: fmt.Sprintf("change_language.%s", langCode),
			},
		)
	}

	return ChunkKeyboardSlices(kb, 2)
}

/*
GetLangFormat returns the formatted language name and flag for a given language code.

Combines the language name and flag emoji for display.
*/
func GetLangFormat(langCode string) string {
	return i18n.I18n{LangCode: langCode}.GetString("main.language_name") +
		" " +
		i18n.I18n{LangCode: langCode}.GetString("main.language_flag")
}

// NOTE: nekobin helper functions

/*
PasteToNekoBin uploads the provided text to Nekobin and returns the paste key.

Truncates text to 65,000 characters if necessary. Returns true and the key on success.
*/
func PasteToNekoBin(text string) (pasted bool, key string) {
	type mapType map[string]interface{}
	var body mapType

	if len(text) > 65000 {
		text = text[:65000]
	}
	postBody, err := json.Marshal(
		map[string]string{
			"content": text,
		},
	)
	if err != nil {
		log.Error(err)
	}

	responseBody := bytes.NewBuffer(postBody)
	resp, err := http.Post("https://nekobin.com/api/documents", "application/json", responseBody)
	if err != nil {
		log.Error(err)
		return
	}
	defer func(Body io.ReadCloser) {
		err = Body.Close()
		if err != nil {
			log.Error(err)
		}
	}(resp.Body)

	err = json.NewDecoder(resp.Body).Decode(&body)
	if err != nil {
		log.Error(err)
		return
	}

	key = body["result"].(map[string]interface{})["key"].(string)
	if key != "" {
		return true, key
	}
	return
}

// NOTE: tgmd2html helper functions

/*
ReverseHTML2MD converts HTML-formatted text to Markdown.

Handles common tags such as <b>, <i>, <u>, <s>, <code>, <pre>, and <a>.
*/
func ReverseHTML2MD(text string) string {
	Html2MdMap := map[string]string{
		"i":    "_%s_",
		"u":    "__%s__",
		"b":    "*%s*",
		"s":    "~%s~",
		"code": "`%s`",
		"pre":  "```%s```",
		"a":    "[%s](%s)",
	}

	for _, i := range strings.Split(text, " ") {
		for htmlTag, keyValue := range Html2MdMap {
			k := ""
			// using this because <a> uses <href> tag
			if htmlTag == "a" {
				re := regexp.MustCompile(`<a href="(.*?)">(.*?)</a>`)
				if re.MatchString(i) {
					k = fmt.Sprintf(keyValue, re.FindStringSubmatch(i)[2], re.FindStringSubmatch(i)[1])
				} else {
					continue
				}
			} else {
				regexPattern := fmt.Sprintf(`<%s>(.*)<\/%s>`, htmlTag, htmlTag)
				pattern := regexp.MustCompile(regexPattern)
				if pattern.MatchString(i) {
					k = fmt.Sprintf(keyValue, pattern.ReplaceAllString(i, "$1"))
				} else {
					continue
				}
			}
			text = strings.ReplaceAll(text, i, k)
		}
	}

	return text
}

// NOTE: formatting helper functions

/*
FormattingReplacer replaces placeholders in a message with user and chat-specific values.

Handles placeholders like {first}, {last}, {fullname}, {username}, {mention}, {count}, {chatname}, and {id}.
Also manages rules button insertion based on chat rules configuration.
*/
func FormattingReplacer(b *gotgbot.Bot, chat *gotgbot.Chat, user *gotgbot.User, oldMsg string, buttons []db.Button) (res string, btns []db.Button) {
	var (
		firstName     string
		fullName      string
		username      string
		rulesBtnRegex = `(?s){rules(:(same|up))?}`
	)

	firstName = user.FirstName
	if len(user.FirstName) <= 0 {
		firstName = "PersonWithNoName"
	}

	if user.LastName != "" {
		fullName = firstName + " " + user.LastName
	} else {
		fullName = firstName
	}
	count, _ := chat.GetMemberCount(b, nil)
	mention := MentionHtml(user.Id, firstName)

	if user.Username != "" {
		username = "@" + html.EscapeString(user.Username)
	} else {
		username = mention
	}
	r := strings.NewReplacer(
		"{first}", html.EscapeString(firstName),
		"{last}", html.EscapeString(user.LastName),
		"{fullname}", html.EscapeString(fullName),
		"{username}", username,
		"{mention}", mention,
		"{count}", strconv.Itoa(int(count)),
		"{chatname}", html.EscapeString(chat.Title),
		"{id}", strconv.Itoa(int(user.Id)),
	)
	res = r.Replace(oldMsg)
	btns = buttons // copies the buttons over to format rules btn

	rulesDb := db.GetChatRulesInfo(chat.Id)
	rulesBtnText := rulesDb.RulesBtn
	if rulesBtnText == "" {
		rulesBtnText = "Rules"
	}

	// only add rules btn when rules are added in chat
	if rulesDb.Rules != "" {
		pattern, err := regexp.Compile(rulesBtnRegex)
		if err != nil {
			log.Error(err)
		}
		if pattern.MatchString(res) {
			response := pattern.FindStringSubmatch(res)

			sameline := false
			if response[2] == "same" {
				sameline = true
			}

			rulesButton := db.Button{
				Name:     rulesBtnText,
				Url:      fmt.Sprintf("https://t.me/%s?start=rules_%d", b.Username, chat.Id),
				SameLine: sameline,
			}

			if response[2] == "up" {
				// this adds the button on top of all buttons
				btns = []db.Button{rulesButton}
				btns = append(btns, buttons...)
			} else {
				// this adds the button to bottom (default behaviour)
				btns = buttons
				btns = append(btns, rulesButton)

			}
			res = pattern.ReplaceAllString(res, "")
		}
	}

	return res, btns
}

// NOTE: extract statis helper functions

/*
ExtractJoinLeftStatusChange determines if a user joined or left a chat based on status change.

Returns two booleans: wasMember and isMember, indicating membership before and after the update.
*/
func ExtractJoinLeftStatusChange(u *gotgbot.ChatMemberUpdated) (bool, bool) {
	// return false for channels
	if u.Chat.Type == "channel" {
		return false, false
	}

	oldMemberStatus := u.OldChatMember.MergeChatMember().Status
	newMemberStatus := u.NewChatMember.MergeChatMember().Status
	oldIsMember := u.OldChatMember.MergeChatMember().IsMember
	newIsMember := u.NewChatMember.MergeChatMember().IsMember

	if oldMemberStatus == newMemberStatus {
		return false, false
	}

	wasMember := string_handling.FindInStringSlice(
		[]string{"member", "administrator", "creator"},
		oldMemberStatus,
	) || (oldMemberStatus == "restricted" && oldIsMember)

	isMember := string_handling.FindInStringSlice(
		[]string{"member", "administrator", "creator"},
		newMemberStatus,
	) || (newMemberStatus == "restricted" && newIsMember)

	return wasMember, isMember
}

/*
ExtractAdminUpdateStatusChange determines if a user's admin status changed in a chat.

Returns true if the user gained or lost admin/creator status, otherwise false.
*/
func ExtractAdminUpdateStatusChange(u *gotgbot.ChatMemberUpdated) bool {
	// return false for channels
	if u.Chat.Type == "channel" {
		return false
	}

	oldMemberStatus := u.OldChatMember.MergeChatMember().Status
	newMemberStatus := u.NewChatMember.MergeChatMember().Status

	// status remains same
	if oldMemberStatus == newMemberStatus {
		return false
	}

	adminStatusChanged := (string_handling.FindInStringSlice(
		[]string{"administrator", "creator"},
		oldMemberStatus,
	) && !string_handling.FindInStringSlice(
		[]string{"administrator", "creator"},
		newMemberStatus,
	)) ||
		(string_handling.FindInStringSlice(
			[]string{"administrator", "creator"},
			newMemberStatus,
		) && !string_handling.FindInStringSlice(
			[]string{"administrator", "creator"},
			oldMemberStatus,
		))

	return adminStatusChanged
}

// NOTE: NoteWelcomeFilter helper functions

/*
GetNoteAndFilterType extracts note or filter data from a message.

Parses the keyword, file ID, text, data type, buttons, and various flags for note/filter configuration.
Returns an error message if the content is invalid.
*/
func GetNoteAndFilterType(msg *gotgbot.Message, isFilter bool) (keyWord, fileid, text string, dataType int, buttons []db.Button, pvtOnly, grpOnly, adminOnly, webPrev, isProtected, noNotif bool, errorMsg string) {
	dataType = -1 // not defined datatype; invalid note
	if isFilter {
		errorMsg = "You need to give the filter some content!"
	} else {
		errorMsg = "You need to give the note some content!"
	}

	var (
		rawText string
		args    = strings.Fields(msg.Text)[1:]
	)
	_buttons := make([]tgmd2html.ButtonV2, 0) // make a slice for buttons
	replyMsg := msg.ReplyToMessage

	// set rawText from helper function
	setRawText(msg, args, &rawText)

	// extract the noteword
	if len(args) >= 2 && replyMsg == nil {
		keyWord, text = extraction.ExtractQuotes(rawText, isFilter, true)
		text, _buttons = tgmd2html.MD2HTMLButtonsV2(text)
		dataType = db.TEXT
	} else if replyMsg != nil && len(args) >= 1 {
		keyWord, _ = extraction.ExtractQuotes(strings.Join(args, " "), isFilter, true)

		if replyMsg.ReplyMarkup == nil {
			text, _buttons = tgmd2html.MD2HTMLButtonsV2(rawText)
		} else {
			text, _ = tgmd2html.MD2HTMLButtonsV2(rawText)
			_buttons = InlineKeyboardMarkupToTgmd2htmlButtonV2(replyMsg.ReplyMarkup)
		}

		if replyMsg.Text != "" {
			dataType = db.TEXT
		} else if replyMsg.Sticker != nil {
			fileid = replyMsg.Sticker.FileId
			dataType = db.STICKER
		} else if replyMsg.Document != nil {
			fileid = replyMsg.Document.FileId
			dataType = db.DOCUMENT
		} else if len(replyMsg.Photo) > 0 {
			fileid = replyMsg.Photo[len(replyMsg.Photo)-1].FileId // using -1 index to get best photo quality
			dataType = db.PHOTO
		} else if replyMsg.Audio != nil {
			fileid = replyMsg.Audio.FileId
			dataType = db.AUDIO
		} else if replyMsg.Voice != nil {
			fileid = replyMsg.Voice.FileId
			dataType = db.VOICE
		} else if replyMsg.Video != nil {
			fileid = replyMsg.Video.FileId
			dataType = db.VIDEO
		} else if replyMsg.VideoNote != nil {
			fileid = replyMsg.VideoNote.FileId
			dataType = db.VideoNote
		}
	}

	// pre-fix the data before sending it back
	preFixes(_buttons, keyWord, &text, &dataType, fileid, &buttons, &errorMsg)

	// return if datatype is invalid
	if dataType != -1 && !isFilter {
		// parse options such as pvtOnly, adminOnly, webPrev and replace them
		pvtOnly, grpOnly, adminOnly, webPrev, isProtected, noNotif, _ = notesParser(text)
	}

	return
}

/*
GetWelcomeType extracts welcome message data from a message.

Parses the text, data type, file ID, buttons, and returns an error message if content is missing.
*/
func GetWelcomeType(msg *gotgbot.Message, greetingType string) (text string, dataType int, fileid string, buttons []db.Button, errorMsg string) {
	dataType = -1
	errorMsg = fmt.Sprintf("You need to give me some content to %s users!", greetingType)
	var (
		rawText string
		args    = strings.Fields(msg.Text)[1:]
	)
	_buttons := make([]tgmd2html.ButtonV2, 0)
	replyMsg := msg.ReplyToMessage

	// set rawText from helper function
	setRawText(msg, args, &rawText)

	if len(args) >= 1 && msg.ReplyToMessage == nil {
		fileid = ""
		text, _buttons = tgmd2html.MD2HTMLButtonsV2(rawText)
		dataType = db.TEXT
	} else if msg.ReplyToMessage != nil {
		if replyMsg.ReplyMarkup == nil {
			text, _buttons = tgmd2html.MD2HTMLButtonsV2(rawText)
		} else {
			text, _ = tgmd2html.MD2HTMLButtonsV2(rawText)
			_buttons = InlineKeyboardMarkupToTgmd2htmlButtonV2(replyMsg.ReplyMarkup)
		}
		if len(args) == 0 && replyMsg.Text != "" {
			dataType = db.TEXT
		} else if replyMsg.Sticker != nil {
			fileid = replyMsg.Sticker.FileId
			dataType = db.STICKER
		} else if replyMsg.Document != nil {
			fileid = replyMsg.Document.FileId
			dataType = db.DOCUMENT
		} else if len(replyMsg.Photo) > 0 {
			fileid = replyMsg.Photo[len(replyMsg.Photo)-1].FileId
			dataType = db.PHOTO
		} else if replyMsg.Audio != nil {
			fileid = replyMsg.Audio.FileId
			dataType = db.AUDIO
		} else if replyMsg.Voice != nil {
			fileid = replyMsg.Voice.FileId
			dataType = db.VOICE
		} else if replyMsg.Video != nil {
			fileid = replyMsg.Video.FileId
			dataType = db.VIDEO
		} else if replyMsg.VideoNote != nil {
			fileid = replyMsg.VideoNote.FileId
			dataType = db.VideoNote
		}
	}

	// pre-fix the data before sending it back
	preFixes(_buttons, "Greeting", &text, &dataType, fileid, &buttons, &errorMsg)

	return
}

/*
SendFilter sends a filter message using the appropriate handler from FiltersEnumFuncMap.

Organizes data and builds the keyboard before sending the message.
*/
func SendFilter(b *gotgbot.Bot, ctx *ext.Context, filterData *db.ChatFilters, replyMsgId int64) (*gotgbot.Message, error) {
	chat := ctx.EffectiveChat

	var (
		buttons       []db.Button
		sent          string
		tmpfilterData db.ChatFilters
	)
	tmpfilterData = *filterData
	buttons = tmpfilterData.Buttons

	rstrings := strings.Split(tmpfilterData.FilterReply, "%%%")
	if len(rstrings) == 1 {
		sent = rstrings[0]
	} else {
		n := rand.Int() % len(rstrings)
		sent = rstrings[n]
	}

	tmpfilterData.FilterReply, buttons = FormattingReplacer(b, chat, ctx.EffectiveUser, sent, buttons)
	keyb := BuildKeyboard(buttons)
	keyboard := gotgbot.InlineKeyboardMarkup{InlineKeyboard: keyb}

	// using false as last arg because we don't want to noformat the message
	msg, err := FiltersEnumFuncMap[tmpfilterData.MsgType](b, ctx, tmpfilterData, &keyboard, replyMsgId, false, filterData.NoNotif)

	return msg, err
}

// notesParser parses note options from the message text.
// Detects flags like {private}, {admin}, {preview}, {noprivate}, {protect}, and {nonotif}.
// Returns corresponding booleans and the cleaned text with flags removed.
func notesParser(sent string) (pvtOnly, grpOnly, adminOnly, webPrev, protectedContent, noNotif bool, sentBack string) {
	pvtOnly, err := regexp.MatchString(`({private})`, sent)
	if err != nil {
		log.Error(err)
		return
	}

	grpOnly, err = regexp.MatchString(`({noprivate})`, sent)
	if err != nil {
		log.Error(err)
		return
	}

	adminOnly, err = regexp.MatchString(`({admin})`, sent)
	if err != nil {
		log.Error(err)
		return
	}

	webPrev, err = regexp.MatchString(`({preview})`, sent)
	if err != nil {
		log.Error(err)
		return
	}

	protectedContent, err = regexp.MatchString(`({protect})`, sent)
	if err != nil {
		log.Error(err)
		return
	}

	noNotif, err = regexp.MatchString(`({nonotif})`, sent)
	if err != nil {
		log.Error(err)
		return
	}

	sent = strings.NewReplacer(
		"{private}", "",
		"{admin}", "",
		"{preview}", "",
		"{noprivate}", "",
		"{protect}", "",
		"{nonotif}", "",
	).Replace(sent)

	return pvtOnly, grpOnly, adminOnly, webPrev, protectedContent, noNotif, sent
}

/*
SendNote sends a note message using the appropriate handler from NotesEnumFuncMap.

Organizes data, builds the keyboard, and formats the note before sending.
Returns the sent message and any error encountered.
*/
func SendNote(b *gotgbot.Bot, chat *gotgbot.Chat, ctx *ext.Context, noteData *db.ChatNotes, replyMsgId int64) (*gotgbot.Message, error) {
	var (
		buttons []db.Button
		sent    string
	)

	// copy just in case
	buttons = noteData.Buttons

	rstrings := strings.Split(noteData.NoteContent, "%%%")
	if len(rstrings) == 1 {
		sent = rstrings[0]
	} else {
		n := rand.Int() % len(rstrings)
		sent = rstrings[n]
	}

	noteData.NoteContent, buttons = FormattingReplacer(b, chat, ctx.EffectiveUser, sent, buttons)
	// below is an additional step, need to remove it
	_, _, _, _, _, _, noteData.NoteContent = notesParser(noteData.NoteContent) // replaces the text
	keyb := BuildKeyboard(buttons)
	keyboard := gotgbot.InlineKeyboardMarkup{InlineKeyboard: keyb}
	// using false as last arg to format the note
	msg, err := NotesEnumFuncMap[noteData.MsgType](b, ctx, noteData, &keyboard, replyMsgId, noteData.WebPreview, noteData.IsProtected, false, noteData.NoNotif)
	// if strings.Contains(err.Error(), "replied message not found") {
	// 	return nil, nil
	// }
	if err != nil {
		log.Error(err)
		return msg, err
	}

	return msg, nil
}

// NotesEnumFuncMap maps data types to their corresponding note sending functions.
// Each function handles sending a note of a specific media type (text, sticker, document, etc.)
// with appropriate formatting, web preview settings, and protection options.
var NotesEnumFuncMap = map[int]func(b *gotgbot.Bot, ctx *ext.Context, noteData *db.ChatNotes, keyb *gotgbot.InlineKeyboardMarkup, replyMsgId int64, webPreview, isProtected bool, noFormat, noNotif bool) (*gotgbot.Message, error){
	db.TEXT: func(b *gotgbot.Bot, ctx *ext.Context, noteData *db.ChatNotes, keyb *gotgbot.InlineKeyboardMarkup, replyMsgId int64, webPreview, isProtected bool, noFormat, noNotif bool) (*gotgbot.Message, error) {
		formatMode := HTML
		if noFormat {
			formatMode = None
		}
		return b.SendMessage(ctx.Message.Chat.Id,
			noteData.NoteContent,
			&gotgbot.SendMessageOpts{
				ParseMode: formatMode,
				LinkPreviewOptions: &gotgbot.LinkPreviewOptions{
					IsDisabled: !webPreview,
				},
				ReplyMarkup: keyb,
				ReplyParameters: &gotgbot.ReplyParameters{
					MessageId:                replyMsgId,
					AllowSendingWithoutReply: true,
				},
				ProtectContent:      isProtected,
				DisableNotification: noNotif,
				MessageThreadId:     ctx.Message.MessageThreadId,
			},
		)
	},
	db.STICKER: func(b *gotgbot.Bot, ctx *ext.Context, noteData *db.ChatNotes, keyb *gotgbot.InlineKeyboardMarkup, replyMsgId int64, _, isProtected bool, _, noNotif bool) (*gotgbot.Message, error) {
		return b.SendSticker(ctx.Message.Chat.Id,
			gotgbot.InputFileByID(noteData.FileID),
			&gotgbot.SendStickerOpts{
				ReplyParameters: &gotgbot.ReplyParameters{
					MessageId:                replyMsgId,
					AllowSendingWithoutReply: true,
				},
				ReplyMarkup:         keyb,
				ProtectContent:      isProtected,
				DisableNotification: noNotif,
				MessageThreadId:     ctx.Message.MessageThreadId,
			},
		)
	},
	db.DOCUMENT: func(b *gotgbot.Bot, ctx *ext.Context, noteData *db.ChatNotes, keyb *gotgbot.InlineKeyboardMarkup, replyMsgId int64, _, isProtected bool, noFormat, noNotif bool) (*gotgbot.Message, error) {
		formatMode := HTML
		if noFormat {
			formatMode = None
		}
		return b.SendDocument(ctx.Message.Chat.Id,
			gotgbot.InputFileByID(noteData.FileID),
			&gotgbot.SendDocumentOpts{
				ReplyParameters: &gotgbot.ReplyParameters{
					MessageId:                replyMsgId,
					AllowSendingWithoutReply: true,
				},
				ParseMode:           formatMode,
				ReplyMarkup:         keyb,
				Caption:             noteData.NoteContent,
				ProtectContent:      isProtected,
				DisableNotification: noNotif,
				MessageThreadId:     ctx.Message.MessageThreadId,
			},
		)
	},
	db.PHOTO: func(b *gotgbot.Bot, ctx *ext.Context, noteData *db.ChatNotes, keyb *gotgbot.InlineKeyboardMarkup, replyMsgId int64, _, isProtected bool, noFormat, noNotif bool) (*gotgbot.Message, error) {
		formatMode := HTML
		if noFormat {
			formatMode = None
		}
		return b.SendPhoto(ctx.Message.Chat.Id,
			gotgbot.InputFileByID(noteData.FileID),
			&gotgbot.SendPhotoOpts{
				ReplyParameters: &gotgbot.ReplyParameters{
					MessageId:                replyMsgId,
					AllowSendingWithoutReply: true,
				},
				ParseMode:           formatMode,
				ReplyMarkup:         keyb,
				Caption:             noteData.NoteContent,
				ProtectContent:      isProtected,
				DisableNotification: noNotif,
				MessageThreadId:     ctx.Message.MessageThreadId,
			},
		)
	},
	db.AUDIO: func(b *gotgbot.Bot, ctx *ext.Context, noteData *db.ChatNotes, keyb *gotgbot.InlineKeyboardMarkup, replyMsgId int64, _, isProtected bool, noFormat, noNotif bool) (*gotgbot.Message, error) {
		formatMode := HTML
		if noFormat {
			formatMode = None
		}
		return b.SendAudio(ctx.Message.Chat.Id,
			gotgbot.InputFileByID(noteData.FileID),
			&gotgbot.SendAudioOpts{
				ReplyParameters: &gotgbot.ReplyParameters{
					MessageId:                replyMsgId,
					AllowSendingWithoutReply: true,
				},
				ParseMode:           formatMode,
				ReplyMarkup:         keyb,
				Caption:             noteData.NoteContent,
				ProtectContent:      isProtected,
				DisableNotification: noNotif,
				MessageThreadId:     ctx.Message.MessageThreadId,
			},
		)
	},
	db.VOICE: func(b *gotgbot.Bot, ctx *ext.Context, noteData *db.ChatNotes, keyb *gotgbot.InlineKeyboardMarkup, replyMsgId int64, _, isProtected bool, noFormat, noNotif bool) (*gotgbot.Message, error) {
		formatMode := HTML
		if noFormat {
			formatMode = None
		}
		return b.SendVoice(ctx.Message.Chat.Id,
			gotgbot.InputFileByID(noteData.FileID),
			&gotgbot.SendVoiceOpts{
				ReplyParameters: &gotgbot.ReplyParameters{
					MessageId:                replyMsgId,
					AllowSendingWithoutReply: true,
				},
				ParseMode:           formatMode,
				ReplyMarkup:         keyb,
				Caption:             noteData.NoteContent,
				ProtectContent:      isProtected,
				DisableNotification: noNotif,
				MessageThreadId:     ctx.Message.MessageThreadId,
			},
		)
	},
	db.VIDEO: func(b *gotgbot.Bot, ctx *ext.Context, noteData *db.ChatNotes, keyb *gotgbot.InlineKeyboardMarkup, replyMsgId int64, _, isProtected bool, noFormat, noNotif bool) (*gotgbot.Message, error) {
		formatMode := HTML
		if noFormat {
			formatMode = None
		}
		return b.SendVideo(ctx.Message.Chat.Id,
			gotgbot.InputFileByID(noteData.FileID),
			&gotgbot.SendVideoOpts{
				ReplyParameters: &gotgbot.ReplyParameters{
					MessageId:                replyMsgId,
					AllowSendingWithoutReply: true,
				},
				ParseMode:           formatMode,
				ReplyMarkup:         keyb,
				Caption:             noteData.NoteContent,
				ProtectContent:      isProtected,
				DisableNotification: noNotif,
				MessageThreadId:     ctx.Message.MessageThreadId,
			},
		)
	},
	db.VideoNote: func(b *gotgbot.Bot, ctx *ext.Context, noteData *db.ChatNotes, keyb *gotgbot.InlineKeyboardMarkup, replyMsgId int64, _, isProtected bool, _, noNotif bool) (*gotgbot.Message, error) {
		return b.SendVideoNote(ctx.Message.Chat.Id,
			gotgbot.InputFileByID(noteData.FileID),
			&gotgbot.SendVideoNoteOpts{
				ReplyParameters: &gotgbot.ReplyParameters{
					MessageId:                replyMsgId,
					AllowSendingWithoutReply: true,
				},
				ReplyMarkup:         keyb,
				ProtectContent:      isProtected,
				DisableNotification: noNotif,
				MessageThreadId:     ctx.Message.MessageThreadId,
			},
		)
	},
}

// GreetingsEnumFuncMap maps data types to their corresponding greeting message sending functions.
// Each function handles sending a welcome/goodbye message of a specific media type.
// FIXME: when using /welcome command in private with connection, the string is sent to connected chat instead of pm.
var GreetingsEnumFuncMap = map[int]func(b *gotgbot.Bot, ctx *ext.Context, msg, fileID string, keyb *gotgbot.InlineKeyboardMarkup) (*gotgbot.Message, error){
	db.TEXT: func(b *gotgbot.Bot, ctx *ext.Context, msg, _ string, keyb *gotgbot.InlineKeyboardMarkup) (*gotgbot.Message, error) {
		return b.SendMessage(
			ctx.EffectiveChat.Id,
			msg,
			&gotgbot.SendMessageOpts{
				ParseMode: HTML,
				LinkPreviewOptions: &gotgbot.LinkPreviewOptions{
					IsDisabled: true,
				},
				ReplyMarkup: keyb,
			},
		)
	},
	db.STICKER: func(b *gotgbot.Bot, ctx *ext.Context, _, fileID string, keyb *gotgbot.InlineKeyboardMarkup) (*gotgbot.Message, error) {
		return b.SendSticker(
			ctx.EffectiveChat.Id,
			gotgbot.InputFileByID(fileID),
			&gotgbot.SendStickerOpts{
				ReplyMarkup:     keyb,
				MessageThreadId: ctx.EffectiveMessage.MessageThreadId,
			},
		)
	},
	db.DOCUMENT: func(b *gotgbot.Bot, ctx *ext.Context, msg, fileID string, keyb *gotgbot.InlineKeyboardMarkup) (*gotgbot.Message, error) {
		return b.SendDocument(
			ctx.EffectiveChat.Id,
			gotgbot.InputFileByID(fileID),
			&gotgbot.SendDocumentOpts{
				ParseMode:       HTML,
				ReplyMarkup:     keyb,
				Caption:         msg,
				MessageThreadId: ctx.EffectiveMessage.MessageThreadId,
			},
		)
	},
	db.PHOTO: func(b *gotgbot.Bot, ctx *ext.Context, msg, fileID string, keyb *gotgbot.InlineKeyboardMarkup) (*gotgbot.Message, error) {
		return b.SendPhoto(
			ctx.EffectiveChat.Id,
			gotgbot.InputFileByID(fileID),
			&gotgbot.SendPhotoOpts{
				ParseMode:       HTML,
				ReplyMarkup:     keyb,
				Caption:         msg,
				MessageThreadId: ctx.EffectiveMessage.MessageThreadId,
			},
		)
	},
	db.AUDIO: func(b *gotgbot.Bot, ctx *ext.Context, msg, fileID string, keyb *gotgbot.InlineKeyboardMarkup) (*gotgbot.Message, error) {
		return b.SendAudio(
			ctx.EffectiveChat.Id,
			gotgbot.InputFileByID(fileID),
			&gotgbot.SendAudioOpts{
				ParseMode:       HTML,
				ReplyMarkup:     keyb,
				Caption:         msg,
				MessageThreadId: ctx.EffectiveMessage.MessageThreadId,
			},
		)
	},
	db.VOICE: func(b *gotgbot.Bot, ctx *ext.Context, msg, fileID string, keyb *gotgbot.InlineKeyboardMarkup) (*gotgbot.Message, error) {
		return b.SendVoice(
			ctx.EffectiveChat.Id,
			gotgbot.InputFileByID(fileID),
			&gotgbot.SendVoiceOpts{
				ParseMode:       HTML,
				ReplyMarkup:     keyb,
				Caption:         msg,
				MessageThreadId: ctx.EffectiveMessage.MessageThreadId,
			},
		)
	},
	db.VIDEO: func(b *gotgbot.Bot, ctx *ext.Context, msg, fileID string, keyb *gotgbot.InlineKeyboardMarkup) (*gotgbot.Message, error) {
		return b.SendVideo(
			ctx.EffectiveChat.Id,
			gotgbot.InputFileByID(fileID),
			&gotgbot.SendVideoOpts{
				ParseMode:       HTML,
				ReplyMarkup:     keyb,
				Caption:         msg,
				MessageThreadId: ctx.EffectiveMessage.MessageThreadId,
			},
		)
	},
	db.VideoNote: func(b *gotgbot.Bot, ctx *ext.Context, _, fileID string, keyb *gotgbot.InlineKeyboardMarkup) (*gotgbot.Message, error) {
		return b.SendVideoNote(
			ctx.EffectiveChat.Id,
			gotgbot.InputFileByID(fileID),
			&gotgbot.SendVideoNoteOpts{
				ReplyMarkup:     keyb,
				MessageThreadId: ctx.EffectiveMessage.MessageThreadId,
			},
		)
	},
}

// FiltersEnumFuncMap maps data types to their corresponding filter message sending functions.
// Each function handles sending a filter response of a specific media type when triggered by matching text.
var FiltersEnumFuncMap = map[int]func(b *gotgbot.Bot, ctx *ext.Context, filterData db.ChatFilters, keyb *gotgbot.InlineKeyboardMarkup, replyMsgId int64, noFormat, noNotif bool) (*gotgbot.Message, error){
	db.TEXT: func(b *gotgbot.Bot, ctx *ext.Context, filterData db.ChatFilters, keyb *gotgbot.InlineKeyboardMarkup, replyMsgId int64, noFormat, noNotif bool) (*gotgbot.Message, error) {
		formatMode := HTML
		if noFormat {
			formatMode = None
		}
		return b.SendMessage(
			ctx.Message.Chat.Id,
			filterData.FilterReply,
			&gotgbot.SendMessageOpts{
				ParseMode: formatMode,
				LinkPreviewOptions: &gotgbot.LinkPreviewOptions{
					IsDisabled: true,
				},
				ReplyParameters: &gotgbot.ReplyParameters{
					MessageId: replyMsgId,
				},
				ReplyMarkup:         keyb,
				DisableNotification: noNotif,
				MessageThreadId:     ctx.Message.MessageThreadId,
			},
		)
	},
	db.STICKER: func(b *gotgbot.Bot, ctx *ext.Context, filterData db.ChatFilters, keyb *gotgbot.InlineKeyboardMarkup, replyMsgId int64, _, noNotif bool) (*gotgbot.Message, error) {
		return b.SendSticker(
			ctx.Message.Chat.Id,
			gotgbot.InputFileByID(filterData.FileID),
			&gotgbot.SendStickerOpts{
				ReplyParameters: &gotgbot.ReplyParameters{
					MessageId: replyMsgId,
				},
				ReplyMarkup:         keyb,
				DisableNotification: noNotif,
				MessageThreadId:     ctx.Message.MessageThreadId,
			},
		)
	},
	db.DOCUMENT: func(b *gotgbot.Bot, ctx *ext.Context, filterData db.ChatFilters, keyb *gotgbot.InlineKeyboardMarkup, replyMsgId int64, noFormat, noNotif bool) (*gotgbot.Message, error) {
		formatMode := HTML
		if noFormat {
			formatMode = None
		}
		return b.SendDocument(
			ctx.Message.Chat.Id,
			gotgbot.InputFileByID(filterData.FileID),
			&gotgbot.SendDocumentOpts{
				ReplyParameters: &gotgbot.ReplyParameters{
					MessageId: replyMsgId,
				},
				ParseMode:           formatMode,
				ReplyMarkup:         keyb,
				Caption:             filterData.FilterReply,
				DisableNotification: noNotif,
				MessageThreadId:     ctx.Message.MessageThreadId,
			},
		)
	},
	db.PHOTO: func(b *gotgbot.Bot, ctx *ext.Context, filterData db.ChatFilters, keyb *gotgbot.InlineKeyboardMarkup, replyMsgId int64, noFormat, noNotif bool) (*gotgbot.Message, error) {
		formatMode := HTML
		if noFormat {
			formatMode = None
		}
		return b.SendPhoto(
			ctx.Message.Chat.Id,
			gotgbot.InputFileByID(filterData.FileID),
			&gotgbot.SendPhotoOpts{
				ReplyParameters: &gotgbot.ReplyParameters{
					MessageId: replyMsgId,
				},
				ParseMode:           formatMode,
				ReplyMarkup:         keyb,
				Caption:             filterData.FilterReply,
				DisableNotification: noNotif,
				MessageThreadId:     ctx.Message.MessageThreadId,
			},
		)
	},
	db.AUDIO: func(b *gotgbot.Bot, ctx *ext.Context, filterData db.ChatFilters, keyb *gotgbot.InlineKeyboardMarkup, replyMsgId int64, noFormat, noNotif bool) (*gotgbot.Message, error) {
		formatMode := HTML
		if noFormat {
			formatMode = None
		}
		return b.SendAudio(
			ctx.Message.Chat.Id,
			gotgbot.InputFileByID(filterData.FileID),
			&gotgbot.SendAudioOpts{
				ReplyParameters: &gotgbot.ReplyParameters{
					MessageId: replyMsgId,
				},
				ParseMode:           formatMode,
				ReplyMarkup:         keyb,
				Caption:             filterData.FilterReply,
				DisableNotification: noNotif,
				MessageThreadId:     ctx.Message.MessageThreadId,
			},
		)
	},
	db.VOICE: func(b *gotgbot.Bot, ctx *ext.Context, filterData db.ChatFilters, keyb *gotgbot.InlineKeyboardMarkup, replyMsgId int64, noFormat, noNotif bool) (*gotgbot.Message, error) {
		formatMode := HTML
		if noFormat {
			formatMode = None
		}
		return b.SendVoice(
			ctx.Message.Chat.Id,
			gotgbot.InputFileByID(filterData.FileID),
			&gotgbot.SendVoiceOpts{
				ReplyParameters: &gotgbot.ReplyParameters{
					MessageId: replyMsgId,
				},
				ParseMode:           formatMode,
				ReplyMarkup:         keyb,
				Caption:             filterData.FilterReply,
				DisableNotification: noNotif,
				MessageThreadId:     ctx.Message.MessageThreadId,
			},
		)
	},
	db.VIDEO: func(b *gotgbot.Bot, ctx *ext.Context, filterData db.ChatFilters, keyb *gotgbot.InlineKeyboardMarkup, replyMsgId int64, noFormat, noNotif bool) (*gotgbot.Message, error) {
		formatMode := HTML
		if noFormat {
			formatMode = None
		}
		return b.SendVideo(
			ctx.Message.Chat.Id,
			gotgbot.InputFileByID(filterData.FileID),
			&gotgbot.SendVideoOpts{
				ReplyParameters: &gotgbot.ReplyParameters{
					MessageId: replyMsgId,
				},
				ParseMode:           formatMode,
				ReplyMarkup:         keyb,
				Caption:             filterData.FilterReply,
				DisableNotification: noNotif,
				MessageThreadId:     ctx.Message.MessageThreadId,
			},
		)
	},
	db.VideoNote: func(b *gotgbot.Bot, ctx *ext.Context, filterData db.ChatFilters, keyb *gotgbot.InlineKeyboardMarkup, replyMsgId int64, _, noNotif bool) (*gotgbot.Message, error) {
		return b.SendVideoNote(
			ctx.Message.Chat.Id,
			gotgbot.InputFileByID(filterData.FileID),
			&gotgbot.SendVideoNoteOpts{
				ReplyParameters: &gotgbot.ReplyParameters{
					MessageId: replyMsgId,
				},
				ReplyMarkup:         keyb,
				DisableNotification: noNotif,
				MessageThreadId:     ctx.Message.MessageThreadId,
			},
		)
	},
}

// preFixes validates and prepares message content and buttons before saving to the database.
// Checks message length constraints, assigns default button names, filters invalid buttons, and trims whitespace.
// Sets error messages and invalidates data if constraints are not met.
func preFixes(buttons []tgmd2html.ButtonV2, defaultNameButton string, text *string, dataType *int, fileid string, dbButtons *[]db.Button, errorMsg *string) {
	if *dataType == db.TEXT && len(*text) > 4096 {
		*dataType = -1
		*errorMsg = fmt.Sprintf("Your message text is %d characters long. The maximum length for text is 4096; please trim it to a smaller size. Note that markdown characters may take more space than expected.", len(*text))
	} else if *dataType != db.TEXT && len(*text) > 1024 {
		*dataType = -1
		*errorMsg = fmt.Sprintf("Your message caption is %d characters long. The maximum caption length is 1024; please trim it to a smaller size. Note that markdown characters may take more space than expected.", len(*text))
	} else {
		for i, button := range buttons {
			if button.Name == "" {
				buttons[i].Name = defaultNameButton
			}
		}

		// temporary variable function until we don't support notes in inline keyboard
		// will remove non url buttons from keyboard
		buttonUrlFixer := func(_buttons *[]tgmd2html.ButtonV2) {
			// regex taken from https://regexr.com/39nr7
			buttonUrlPattern, _ := regexp.Compile(`[(htps)?:/w.a-zA-Z\d@%_+~#=]{2,256}\.[a-z]{2,6}\b([-a-zA-Z\d@:%_+.~#?&/=]*)`)
			buttons = *_buttons
			for i, btn := range *_buttons {
				if !buttonUrlPattern.MatchString(btn.Content) {
					buttons = append(buttons[:i], buttons[i+1:]...)
				}
			}
			*_buttons = buttons
		}

		buttonUrlFixer(&buttons)
		*dbButtons = ConvertButtonV2ToDbButton(buttons)

		// trim the characters \n, \t, \r and space from the text
		// also, set the dataType to -1 to make note invalid
		*text = strings.Trim(*text, "\n\t\r ")
		if *text == "" && fileid == "" {
			*dataType = -1
		}
	}
}

// setRawText extracts the raw text content from a gotgbot.Message.
// Handles both direct messages and replies, considering text and caption fields.
// Extracts content using the appropriate markdown format preserving original formatting.
func setRawText(msg *gotgbot.Message, args []string, rawText *string) {
	replyMsg := msg.ReplyToMessage
	if replyMsg == nil {
		if msg.Text == "" && msg.Caption != "" {
			*rawText = strings.SplitN(msg.OriginalCaptionMDV2(), " ", 2)[1] // using [1] to remove the command
		} else if msg.Text != "" && msg.Caption == "" {
			*rawText = strings.SplitN(msg.OriginalMDV2(), " ", 2)[1] // using [1] to remove the command
		}
	} else {
		if replyMsg.Text == "" && replyMsg.Caption != "" {
			*rawText = replyMsg.OriginalCaptionMDV2()
		} else if replyMsg.Caption == "" && len(args) >= 2 {
			*rawText = strings.SplitN(msg.OriginalMDV2(), " ", 3)[2] // using [1] to remove the command
		} else {
			*rawText = replyMsg.OriginalMDV2()
		}
	}
}
