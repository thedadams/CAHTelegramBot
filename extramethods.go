package main

import (
	"cahbot/secrets"
	"cahbot/tgbotapi"
	"crypto/sha512"
	"encoding/base64"
	"html"
	"log"
	"strconv"
	"strings"
)

// This is the starting point for handling an update from chat.
func (bot *CAHBot) HandleUpdate(update *tgbotapi.Update) {
	bot.AddUserToDatabase(update.Message.From, update.Message.Chat.ID)
	GameID, Response, err := GetGameID(update.Message.From.ID, bot.db_conn)
	messageType := bot.DetectKindMessageRecieved(&update.Message)
	log.Printf("[%s] Message type: %s", update.Message.From.UserName, messageType)
	if messageType == "command" {
		bot.ProccessCommand(&update.Message, GameID)
	} else if messageType == "message" && Response != "" {
		switch Response {
		case "settings":
			// Handle the change of a setting here.
		case "answer":
			// Handle the receipt of an answer here.
			answer := AnswerIsValid(bot, update.Message.From.ID, update.Message.Text)
			if answer == -1 {
				log.Printf("The text we received was not a valid answer.  We assume it was a message to the game so we are forwarding it.")
				bot.ForwardMessageToGame(&update.Message, GameID)
			} else {
				bot.RecievedAnswerFromPlayer(update.Message.From.ID, GameID, answer)
			}
		case "tradeincard":
			// Handle the trading in of a card here.
		case "czarbest":
			// Handle the receipt of a czar picking best answer here.
		case "czarworst":
			// Handle the receipt of a czar picking the worst answer here.
		}
	} else if messageType == "message" || messageType == "photo" || messageType == "video" || messageType == "audio" || messageType == "contact" || messageType == "document" || messageType == "location" || messageType == "sticker" {
		if err != nil {
			bot.SendMessage(tgbotapi.NewMessage(update.Message.Chat.ID, "It seems that you are not involved in any game so your message fell on deaf ears."))
		} else {
			bot.ForwardMessageToGame(&update.Message, GameID)
		}
	}
}

// This method forwards a message from a player to the rest of the group.
func (bot *CAHBot) SendMessageToGame(GameID, message string) {
	tx, err := bot.db_conn.Begin()
	defer tx.Rollback()
	if err != nil {
		log.Printf("ERROR: %v", err)
		return
	}
	rows, err := bot.db_conn.Query("SELECT get_userids_for_game($1)", GameID)
	defer rows.Close()
	if err != nil {
		log.Printf("ERROR: %v", err)
		return
	}
	var ID int
	for rows.Next() {
		if err := rows.Scan(&ID); err != nil {
			log.Printf("ERROR: %v", err)
		} else {
			bot.SendMessage(tgbotapi.NewMessage(ID, message))
		}
	}
	if err := rows.Err(); err != nil {
		log.Printf("ERROR: %v", err)
	}
}

// This method forwards a message from a player to the rest of the group.
func (bot *CAHBot) ForwardMessageToGame(m *tgbotapi.Message, GameID string) {
	tx, err := bot.db_conn.Begin()
	defer tx.Rollback()
	if err != nil {
		log.Printf("ERROR: %v", err)
		bot.SendActionFailedMessage(m.From.ID)
		return
	}
	rows, err := bot.db_conn.Query("SELECT get_userids_for_game($1)", GameID)
	defer rows.Close()
	if err != nil {
		log.Printf("ERROR: %v", err)
		bot.SendActionFailedMessage(m.From.ID)
		return
	}
	var ID int
	for rows.Next() {
		if err := rows.Scan(&ID); err != nil {
			log.Printf("ERROR: %v", err)
		} else if ID != m.From.ID {
			bot.ForwardMessage(tgbotapi.NewForward(ID, m.Chat.ID, m.MessageID))
		}
	}
	if err := rows.Err(); err != nil {
		log.Printf("ERROR: %v", err)
	}
}

// Send a 'There is no game' message
func (bot *CAHBot) SendNoGameMessage(ChatID int) {
	bot.SendMessage(tgbotapi.NewMessage(ChatID, "You are currently not in a game.  Use command '/create' to create a new one or '/join <id>' to join a game with an id."))
}

func (bot *CAHBot) WrongCommand(ChatID int) {
	bot.SendMessage(tgbotapi.NewMessage(ChatID, "Sorry, I don't know that command."))
}

// This method sends a generic sorry message.
func (bot *CAHBot) SendActionFailedMessage(ChatID int) {
	bot.SendMessage(tgbotapi.NewMessage(ChatID, "I'm sorry, but it seems I have have difficulties right now.  You can try again later or contact my developer @thedadams."))
}

// Here we detect the kind of message we received from the user.
func (bot *CAHBot) DetectKindMessageRecieved(m *tgbotapi.Message) string {
	log.Printf("Detecting the type of message received")
	if m.Text != "" {
		if strings.HasPrefix(m.Text, "/") {
			return "command"
		} else {
			return "message"
		}
	}
	if len(m.Photo) != 0 {
		return "photo"
	}
	if m.Audio.FileID != "" {
		return "audio"
	}
	if m.Video.FileID != "" {
		return "video"
	}
	if m.Document.FileID != "" {
		return "document"
	}
	if m.Sticker.FileID != "" {
		return "sticker"
	}
	if m.NewChatParticipant.ID != 0 {
		return "newparicipant"
	}
	if m.LeftChatParticipant.ID != 0 {
		return "byeparticipant"
	}
	if m.NewChatTitle != "" {
		return "newchattitle"
	}
	if len(m.NewChatPhoto) != 0 {
		return "newchatphoto"
	}
	if m.DeleteChatPhoto {
		return "deletechatphoto"
	}
	if m.GroupChatCreated {
		return "newgroupchat"
	}
	if m.Contact.UserID != "" || m.Contact.FirstName != "" || m.Contact.LastName != "" {
		return "contact"
	}
	if m.Location.Longitude != 0 && m.Location.Latitude != 0 {
		return "location"
	}

	return "undetermined"
}

// Here, we know we have a command, we figure out which command the user invoked,
// and call the appropriate method.
func (bot *CAHBot) ProccessCommand(m *tgbotapi.Message, GameID string) {
	log.Printf("Processing command....")
	// Get the command.
	switch strings.ToLower(strings.Replace(strings.Fields(m.Text)[0], "/", "", 1)) {
	case "start":
		bot.SendMessage(tgbotapi.NewMessage(m.Chat.ID, "Welcome to Cards Against Humanity for Telegram.  To create a new game, use the command '/create'.  If you create a game, you will be given a 5 character id you can share with friends so they can join you.  You can also join a game using the '/join <id>' command where the '<id>' is replaced with a game id created by someone else.  To see all available commands, use '/help'."))
		bot.SendMessage(tgbotapi.NewMessage(m.Chat.ID, "While you are in a game, any (non-command) message you send to me will be automatically forwarded to everyone else in the game so you're all in the loop."))
	case "help":
		// TODO: use helpers to build a help message.
		bot.SendMessage(tgbotapi.NewMessage(m.Chat.ID, "A help message should go here."))
	case "create":
		if GameID != "" {
			bot.SendMessage(tgbotapi.NewMessage(m.Chat.ID, "You are already part of a game with id "+GameID+" and cannot create another game.  You can leave your current game with the command '/leave'."))
		} else {
			ID := bot.CreateNewGame(m.Chat.ID, m.From)
			if ID != "" {
				bot.SendMessage(tgbotapi.NewMessage(m.Chat.ID, "The game was created successfully.  Tell your friends to use the command '/join "+ID+"' to join your game.  Remember that your game will be deleted after 2 days of inactivity."))
				bot.AddPlayerToGame(ID, m.From)
			} else {
				bot.SendMessage(tgbotapi.NewMessage(m.Chat.ID, "An error occurred while trying to create the game.  The game was not created."))
			}
		}
	case "remove":
		tx, err := bot.db_conn.Begin()
		defer tx.Rollback()
		if err != nil {
			log.Printf("ERROR: %v", err)
			bot.SendActionFailedMessage(m.Chat.ID)
			return
		}
		// If the user is in a game, we remove them.
		if GameID != "" {
			bot.RemovePlayerFromGame(GameID, m.From)
		}
		log.Printf("Removing user from the database.")
		_, err = tx.Exec("SELECT remove_user($1)", m.From.ID)
		if err != nil {
			log.Printf("ERROR: %v", err)
			bot.SendActionFailedMessage(m.Chat.ID)
			return
		}
		tx.Commit()
		bot.SendMessage(tgbotapi.NewMessage(m.Chat.ID, "You have been removed from our records. If you ever want to come back, send the command '/start'.  Thank you for playing."))

	case "begin":
		if GameID != "" {
			bot.BeginGame(GameID)
		} else {
			bot.SendNoGameMessage(m.Chat.ID)
		}
	case "end":
		if GameID != "" {
			bot.EndGame(GameID, m.From)
		} else {
			bot.SendNoGameMessage(m.Chat.ID)
		}
	case "join":
		if len(strings.Fields(m.Text)) > 1 {
			if GameID != "" {
				bot.SendMessage(tgbotapi.NewMessage(m.Chat.ID, "You are already part of a game with id "+GameID+" and cannot join another game.  You can leave your current game with the command '/leave'."))
			} else {
				// If the user is not part of another game, we check to see if they id they
				// game is valid.
				tx, err := bot.db_conn.Begin()
				defer tx.Rollback()
				if err != nil {
					log.Printf("ERROR: %v", err)
					bot.SendActionFailedMessage(m.Chat.ID)
					return
				}
				var exists bool
				row := tx.QueryRow("SELECT check_game_exists($1)", strings.Fields(m.Text)[1])
				if _ = row.Scan(&exists); exists {
					// The id is valid and we add them.
					bot.AddPlayerToGame(strings.Fields(m.Text)[1], m.From)
				} else {
					bot.SendMessage(tgbotapi.NewMessage(m.Chat.ID, "There is no game with id "+strings.Fields(m.Text)[1]+".  Please try again with a new id or use '/create' to create a game."))
				}
			}
		} else {
			bot.SendMessage(tgbotapi.NewMessage(m.Chat.ID, "You did not enter a game id.  Try again with the format '/join <id>'."))
		}
	case "gameid":
		if GameID != "" {
			bot.SendMessage(tgbotapi.NewMessage(m.Chat.ID, "The game you are currently playing has id "+GameID+".  Others can join your game by using the command '/join "+GameID+"'."))
		} else {
			bot.SendNoGameMessage(m.Chat.ID)
		}
	case "leave":
		if GameID != "" {
			bot.RemovePlayerFromGame(GameID, m.From)
		} else {
			bot.SendNoGameMessage(m.Chat.ID)
		}
	case "next":
		if GameID != "" {
			bot.StartRound(GameID)
		} else {
			bot.SendNoGameMessage(m.Chat.ID)
		}
	case "mycards":
		if GameID != "" {
			bot.ListCardsForUserWithMessage(GameID, m.From.ID, "Your cards are listed in the keyboard area.")
		} else {
			bot.SendNoGameMessage(m.Chat.ID)
		}
	case "scores":
		if GameID != "" {
			bot.SendMessage(tgbotapi.NewMessage(m.Chat.ID, "Here are the current scores:\n"+GameScores(GameID, bot.db_conn)))
		} else {
			bot.SendNoGameMessage(m.Chat.ID)
		}
	case "settings":
		if GameID != "" {
			bot.SendGameSettings(GameID, m.Chat.ID)
		} else {
			bot.SendNoGameMessage(m.Chat.ID)
		}
	case "changesettings":
		if GameID != "" {
			bot.ChangeGameSettings(GameID)
		} else {
			bot.SendNoGameMessage(m.Chat.ID)
		}
	case "whoisczar":
		if GameID != "" {
			var czar string
			tx, err := bot.db_conn.Begin()
			defer tx.Rollback()
			if err != nil {
				log.Printf("ERROR: %v", err)
				bot.SendActionFailedMessage(m.Chat.ID)
				return
			}
			err = tx.QueryRow("SELECT whoisczar($1)", GameID).Scan(&czar)
			if err != nil {
				log.Printf("ERROR: %v", err)
				bot.SendActionFailedMessage(m.Chat.ID)
				return
			}
			bot.SendMessage(tgbotapi.NewMessage(m.Chat.ID, "The current Card czar is "+czar+"."))
		} else {
			bot.SendNoGameMessage(m.Chat.ID)
		}
	case "logging":
		if len(strings.Fields(m.Text)) > 1 {
			hasher := sha512.New()
			if strings.EqualFold(base64.URLEncoding.EncodeToString(hasher.Sum([]byte(strings.Fields(m.Text)[1]))), secrets.AppPass) {
				bot.Debug = !bot.Debug
				log.Printf("Debugging/verbose logging has been turned to %v.", bot.Debug)
			}
		} else {
			bot.WrongCommand(m.Chat.ID)
		}
	default:
		bot.WrongCommand(m.Chat.ID)
	}
}

// Add a player to a game if the player is not playing.
func (bot *CAHBot) AddPlayerToGame(GameID string, User tgbotapi.User) {
	// This is supposed to check that there are not more than 10 players in a game.
	tx, err := bot.db_conn.Begin()
	defer tx.Rollback()
	if err != nil {
		log.Printf("ERROR: %v", err)
		bot.SendActionFailedMessage(User.ID)
		return
	}
	var numPlayersInGame int
	err = tx.QueryRow("SELECT num_players_in_game($1)", GameID).Scan(&numPlayersInGame)
	if numPlayersInGame > 10 {
		bot.SendMessage(tgbotapi.NewMessage(User.ID, "Player limit of 10 reached, we can not add any more players."))
	} else {
		var tmp bool
		row := tx.QueryRow("SELECT is_player_in_game($2,$1)", GameID, User.ID)
		if _ = row.Scan(&tmp); tmp {
			bot.SendMessage(tgbotapi.NewMessage(User.ID, "You are already playing in this game.  Use command '/leave' to remove yourself."))
		} else {
			log.Printf("Adding %v to the game %v...", User, GameID)
			_, err = tx.Exec("SELECT add_player_to_game($1, $2)", GameID, User.ID)
			if err != nil {
				log.Printf("ERROR: %v", err)
				bot.SendActionFailedMessage(User.ID)
				return
			}
			if tx.Commit() != nil {
				log.Printf("ERROR %T: %v", err, err)
				bot.SendActionFailedMessage(User.ID)
				return
			}
			bot.SendMessageToGame(GameID, User.String()+" has joined the game!")
		}
	}
}

// This method adds a user to the database. It does not link them to a game.
func (bot *CAHBot) AddUserToDatabase(User tgbotapi.User, ChatID int) bool {
	// Check to see if the user is already in the database.
	var exists bool
	tx, err := bot.db_conn.Begin()
	defer tx.Rollback()
	if err != nil {
		log.Printf("Cannot connect to the database.")
		return false
	}
	_ = tx.QueryRow("SELECT does_user_exist($1)", User.ID).Scan(&exists)
	if !exists {
		log.Printf("Adding user with ID %v to the database.", User.ID)
		_, err = tx.Exec("SELECT add_user($1,$2,$3,$4,$5)", User.ID, User.FirstName, User.LastName, User.UserName, User.String())
		if err != nil {
			log.Printf("ERROR: %v", err)
			bot.SendActionFailedMessage(User.ID)
			return false
		}
		tx.Commit()
		return true
	} else {
		log.Printf("User with id %v is already in the database.", User.ID)
	}
	return true
}

// This method begins an already created game.
func (bot *CAHBot) BeginGame(GameID string) {
	tx, err := bot.db_conn.Begin()
	defer tx.Rollback()
	if err != nil {
		log.Printf("ERROR: %v", err)
		bot.SendMessageToGame(GameID, "We could not start the game because of an internal error.")
		return
	}
	// Check to see if there are more than 2 players.
	var tmp int
	err = tx.QueryRow("SELECT num_players_in_game($1)", GameID).Scan(&tmp)
	if err != nil {
		log.Printf("ERROR: %v", err)
		bot.SendMessageToGame(GameID, "We could not start the game because of an internal error.")
		return
	}
	if tmp < 2 {
		log.Printf("There aren't enough players in game with id " + GameID + " to start it.")
		tx.Rollback()
		bot.SendMessageToGame(GameID, "You really need at least 3 players to make it interesting.  Right now, you have "+strconv.Itoa(tmp)+".  Tell others to use the command '/join "+GameID+"' to join your game.")
		return
	}
	log.Printf("Trying to start game with id %v.", GameID)
	tx.Commit()
	bot.SendMessageToGame(GameID, "Get ready, we are starting the game!")
	bot.StartRound(GameID)
}

func (bot *CAHBot) ChangeGameSettings(GameID string) {

}

// This method creates a new game.
func (bot *CAHBot) CreateNewGame(ChatID int, User tgbotapi.User) string {
	tx, err := bot.db_conn.Begin()
	defer tx.Rollback()
	if err != nil {
		log.Printf("ERROR: %v", err)
		bot.SendActionFailedMessage(ChatID)
		return ""
	}
	var GameID string
	for {
		GameID = GetRandomID()
		var tmp bool
		_ = tx.QueryRow("SELECT check_game_exists($1)", GameID).Scan(&tmp)
		if !tmp {
			break
		}
	}
	log.Printf("Creating a new game with ID %v.", GameID)
	// Get the keys for the All Cards map.SE
	ShuffledQuestionCards := make([]int, len(bot.AllQuestionCards))
	for i := 0; i < len(ShuffledQuestionCards); i++ {
		ShuffledQuestionCards[i] = i
	}
	ShuffledAnswerCards := make([]int, len(bot.AllAnswerCards))
	for i := 0; i < len(ShuffledAnswerCards); i++ {
		ShuffledAnswerCards[i] = i
	}
	if err != nil {
		log.Printf("Error creating game: %v", err)
		bot.SendActionFailedMessage(ChatID)
		return ""
	}
	tx.Exec("SELECT add_game($1,$2,$3,$4)", GameID, ArrayTransforForPostgres(ShuffledQuestionCards), ArrayTransforForPostgres(ShuffledAnswerCards), User.ID)
	err = tx.Commit()
	if err != nil {
		log.Printf("Game could not be created. ERROR: %v", err)
		bot.SendActionFailedMessage(ChatID)
		return ""
	}
	log.Printf("Game with id %v created successfully!", GameID)
	return GameID
}

// Sends a message show the players the question card.
func (bot *CAHBot) DisplayQuestionCard(GameID string, AddCardsToPlayersHands bool) {
	log.Printf("Getting question card index for game with id %v", GameID)
	tx, err := bot.db_conn.Begin()
	defer tx.Rollback()
	if err != nil {
		log.Printf("ERROR: %v", err)
		return
	}
	var index int
	err = tx.QueryRow("SELECT get_question_card($1)", GameID).Scan(&index)
	if err != nil {
		log.Printf("ERROR: %v", err)
		return
	}
	log.Printf("The current question cards for game with id %v has index %v.", GameID, index)
	if AddCardsToPlayersHands && bot.AllQuestionCards[index].NumAnswers > 1 {
		_, err = tx.Exec("SELECT add_cards_to_all_in_game($1, $2)", GameID, bot.AllQuestionCards[index].NumAnswers-1)
		if err != nil {
			log.Printf("ERROR: %v", err)
			return
		}
	}
	tx.Commit()
	log.Printf("Sending question card to game with ID %v...", GameID)
	var message string = "Here is the question card:\n\n"
	message += bot.AllQuestionCards[index].Text
	bot.SendMessageToGame(GameID, html.UnescapeString(message))
}

// This method stops and ends an already created game.
func (bot *CAHBot) EndGame(GameID string, User tgbotapi.User) {
	tx, err := bot.db_conn.Begin()
	defer tx.Rollback()
	if err != nil {
		log.Printf("ERROR: %v", err)
		bot.SendMessageToGame(GameID, "There was an error when I tried to end the game.  You can try again or contact my developer @thedadams.")
		return
	}
	log.Printf("Deleting a game with id %v...", GameID)
	rows, err := tx.Query("SELECT end_game($1)", GameID)
	defer rows.Close()
	if err != nil {
		log.Printf("ERROR: %v", err)
		bot.SendMessageToGame(GameID, "There was an error when I tried to end the game.  You can try again or contact my developer @thedadams.")
		return
	}
	if User.ID != -1 {
		// Someone ended the game.
		bot.SendMessageToGame(GameID, "The game has been stopped "+User.String()+".  Here are the scores:\n"+BuildScoreList(rows)+"Thanks for playing!")
	} else {
		// The game ended because someone won.
		bot.SendMessageToGame(GameID, "The game has ended.  Here are the scores:\n"+BuildScoreList(rows)+"Thanks for playing!")
	}
	tx.Commit()
}

// This method lists the answers for everyone and allows the czar to choose one.
func (bot *CAHBot) ListAnswers(GameID string) {
	tx, err := bot.db_conn.Begin()
	defer tx.Rollback()
	if err != nil {
		log.Printf("ERROR: %v", err)
		return
	}
	var cards string
	err = tx.QueryRow("SELECT get_answers($1)", GameID).Scan(&cards)
	if err != nil {
		log.Printf("ERROR: %v", err)
		return
	}
	text := "Here are the submitted answers:\n\n"
	cardsKeyboard := make([][]string, 1)
	for i, val := range ShuffleAnswers(strings.Split(cards[1:len(cards)-1], "+=+\",")) {
		text += html.UnescapeString(val[1:]) + "\n"
		cardsKeyboard[i] = make([]string, 1)
		cardsKeyboard[i][0] = html.UnescapeString(val[1 : len(val)-1])
	}
	log.Printf("Showing everyone the answers submitted for game %v.", GameID)
	bot.SendMessageToGame(GameID, text)
	var czarID int
	err = tx.QueryRow("SELECT czar_id($1, $2)", GameID, "czarbest").Scan(&czarID)
	if err != nil {
		log.Printf("ERROR: %v", err)
		return
	}
	log.Printf("Asking the czar, %v, to pick an answer for game with id %v.", czarID, GameID)
	message := tgbotapi.NewMessage(czarID, "Czar, please choose the best answer.")
	message.ReplyMarkup = tgbotapi.ReplyKeyboardMarkup{cardsKeyboard, true, true, false}
	bot.SendMessage(message)
}

// This method lists a user's cards using a custom keyboard in the Telegram API.  If we need them to respond to a question, this is handled.
func (bot *CAHBot) ListCardsForUserWithMessage(GameID string, UserID int, text string) {
	log.Printf("Showing the user %v their cards.", UserID)
	tx, err := bot.db_conn.Begin()
	defer tx.Rollback()
	if err != nil {
		log.Printf("ERROR: %v", err)
		bot.SendActionFailedMessage(UserID)
		return
	}
	var response string
	err = tx.QueryRow("SELECT get_user_cards($1)", UserID).Scan(&response)
	if err != nil {
		log.Printf("ERROR: %v", err)
		bot.SendActionFailedMessage(UserID)
		return
	}
	response = response[1 : len(response)-1]
	message := tgbotapi.NewMessage(UserID, text)
	cards := make([][]string, len(strings.Split(response, ",")))
	for i := range cards {
		cards[i] = make([]string, 1)
	}
	for i := 0; i < len(strings.Split(response, ",")); i++ {
		tmp, _ := strconv.Atoi(strings.Split(response, ",")[i])
		cards[i][0] = html.UnescapeString(bot.AllAnswerCards[tmp].Text)
	}
	message.ReplyMarkup = tgbotapi.ReplyKeyboardMarkup{cards, true, true, false}
	bot.SendMessage(message)
}

// Handle the receipt of an answer from a player.
func (bot *CAHBot) RecievedAnswerFromPlayer(UserID int, GameID string, AnswerIndex int) {
	tx, err := bot.db_conn.Begin()
	defer tx.Rollback()
	if err != nil {
		log.Printf("ERROR: %v", err)
		bot.SendActionFailedMessage(UserID)
		return
	}
	var QuestionIndex int
	var DisplayName string
	var CurrentAnswer string
	err = tx.QueryRow("SELECT get_question_card($1)", GameID).Scan(&QuestionIndex)
	if err != nil {
		log.Printf("ERROR: %v", err)
		bot.SendActionFailedMessage(UserID)
		return
	}
	CurrentAnswer = strings.Replace(bot.AllQuestionCards[QuestionIndex].Text, "_", bot.AllAnswerCards[AnswerIndex].Text, 1)
	_, err = tx.Exec("SELECT received_answer_from_user($1, $2, $3, $4)", UserID, AnswerIndex, CurrentAnswer, strings.Contains(CurrentAnswer, "_"))
	if err != nil {
		log.Printf("ERROR: %v", err)
		bot.SendActionFailedMessage(UserID)
		return
	}
	err = tx.QueryRow("SELECT get_display_name($1)", UserID).Scan(&DisplayName)
	if err != nil {
		log.Printf("ERROR: %v", err)
		bot.SendActionFailedMessage(UserID)
		return
	}
	if strings.Contains(CurrentAnswer, "_") {
		log.Printf("We received a valid answer from user with id %v, but we need another answer.", UserID)
		bot.ListCardsForUserWithMessage(GameID, UserID, "We received your answer, but this is a multi-answer questions.  Please choose another answer.")
	} else {
		log.Printf("We received a valid, complete answer from user with id %v.", UserID)
		bot.SendMessageToGame(GameID, "We received "+DisplayName+"'s answer.")
		err = tx.QueryRow("SELECT do_we_have_all_answers($1)", GameID).Scan(&QuestionIndex)
		if err != nil {
			log.Printf("ERROR: %v", err)
			bot.SendActionFailedMessage(UserID)
			return
		}
		if QuestionIndex == 1 {
			go bot.ListAnswers(GameID)
		}
	}
	tx.Commit()
}

// Remove a player from a game if the player is playing.
func (bot *CAHBot) RemovePlayerFromGame(GameID string, User tgbotapi.User) {
	tx, err := bot.db_conn.Begin()
	defer tx.Rollback()
	if err != nil {
		log.Printf("ERROR: %v", err)
		bot.SendActionFailedMessage(User.ID)
		return
	}
	log.Printf("Removing %v from the game %v...", User, GameID)
	var str string = ""
	err = tx.QueryRow("SELECT remove_player_from_game($1)", User.ID).Scan(&str)
	if err != nil {
		log.Printf("ERROR: %v", err)
		bot.SendActionFailedMessage(User.ID)
		return
	}
	bot.SendMessage(tgbotapi.NewMessage(User.ID, "Thanks for playing, "+User.String()+"!  You collected "+strings.Split(str[1:len(str)-1], ",")[1]+" Awesome Points."))
	// Now check to see if there is anyone still in the game.
	var numPlayersInGame int
	err = tx.QueryRow("SELECT num_players_in_game($1)", GameID).Scan(&numPlayersInGame)
	if err != nil {
		log.Printf("ERROR: %v", err)
		tx.Commit()
		return
	}
	tx.Commit()
	if numPlayersInGame == 0 {
		log.Printf("There are no more players in game with id %v.  We shall end it.", GameID)
		bot.EndGame(GameID, User)
	} else {
		bot.SendMessageToGame(GameID, User.String()+" has left the game with a score of "+strings.Split(str[1:len(str)-1], ",")[1]+".")
	}
}

func (bot *CAHBot) SendGameSettings(GameID string, ChatID int) {
	tx, err := bot.db_conn.Begin()
	if err != nil {
		log.Printf("ERROR: %v", err)
		bot.SendActionFailedMessage(ChatID)
		tx.Rollback()
		return
	}
	// This is really bad, but I want to see if it works.
	var settings string
	err = tx.QueryRow("SELECT game_settings($1)", GameID).Scan(&settings)
	tx.Rollback()
	if err != nil {
		log.Printf("ERROR: %v", err)
		return
	}
	var text string = "Game settings:\n"
	settings = strings.Replace(settings, "false", "No", -1)
	settings = strings.Replace(settings, "true", "Yes", -1)
	for _, val := range strings.Split(settings[1:len(settings)-1], ",") {
		text += val[1:len(val)-1] + "\n"
	}
	log.Printf("Sending game settings for %v.", GameID)
	bot.SendMessage(tgbotapi.NewMessage(ChatID, text))
}

// This method handles the starting/resuming of a round.
func (bot *CAHBot) StartRound(GameID string) {
	tx, err := bot.db_conn.Begin()
	defer tx.Rollback()
	if err != nil {
		log.Printf("ERROR: %v", err)
		bot.SendMessageToGame(GameID, "We ran into an error and cannot start the next round.  You can report the error to my developer, @thedadams, or try again later.")
		return
	}
	// Check to see if the game is running and if we are waiting for answers.
	var waiting bool
	err = tx.QueryRow("SELECT waiting_for_answers($1)", GameID).Scan(&waiting)
	if err != nil {
		log.Printf("ERROR: %v", err)
		bot.SendMessageToGame(GameID, "We ran into an error and cannot start the next round.  You can report the error to my developer, @thedadams, or try again later.")
		return
	}
	if waiting {
		bot.SendMessageToGame(GameID, "We are waiting for players to give answers.")
	} else {
		rows, err := tx.Query("SELECT start_round($1)", GameID)
		defer rows.Close()
		if err != nil {
			log.Printf("ERROR: %v", err)
			bot.SendMessageToGame(GameID, "We ran into an error and cannot start the next round.  You can report the error to my developer, @thedadams, or try again later.")
			return
		}
		tx.Commit()
		bot.DisplayQuestionCard(GameID, true)
		for rows.Next() {
			var id int
			err = rows.Scan(&id)
			if err != nil {
				log.Printf("ERROR: %v", err)
			} else {
				log.Printf("Asking %v for an answer card.", id)
				bot.ListCardsForUserWithMessage(GameID, id, "Please pick an answer for the question.")
			}
		}
	}
}

// This method handles the czar choosing an answer.
func (bot *CAHBot) CzarChoseAnswer(GameID string) {

}
