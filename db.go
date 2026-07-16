package main

import (
	"database/sql"
	"errors"
	"strconv"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const dbFile = "kidmoney.db"
const weeklyBonusDescription = "Бонус недели"

var DB *sql.DB

var (
	ErrGoalNotFound        = errors.New("goal not found")
	ErrGoalAlreadyBought   = errors.New("goal already bought")
	ErrNotEnoughBalance    = errors.New("not enough balance")
	ErrTaskNotFound        = errors.New("task not found")
	ErrTaskAlreadyComplete = errors.New("task already complete")
	ErrTaskNotPending      = errors.New("task not pending")
	ErrTaskNotActive       = errors.New("task not active")
	ErrNotEnoughSafeMoney  = errors.New("not enough safe money")
	ErrWheelAlreadySpun    = errors.New("wheel already spun today")
)

type Setting struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type HistoryRecord struct {
	ID          int       `json:"id"`
	Description string    `json:"description"`
	Amount      int       `json:"amount"`
	CreatedAt   time.Time `json:"created_at"`
}

type WeeklyBonusResult struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Awarded bool   `json:"awarded"`
	Amount  int    `json:"amount"`
	Balance int    `json:"balance"`
}

type Goal struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Cost       int    `json:"cost"`
	IsAchieved bool   `json:"is_achieved"`
}

type Task struct {
	ID     int    `json:"id"`
	Title  string `json:"title"`
	Reward int    `json:"reward"`
	Status string `json:"status"`
}

type SafeInfo struct {
	Balance int `json:"balance"`
}

type MoneyMoveResult struct {
	Balance     int `json:"balance"`
	SafeBalance int `json:"safe_balance"`
	Amount      int `json:"amount"`
}

type WheelStatus struct {
	CanSpin      bool   `json:"can_spin"`
	LastSpinDate string `json:"last_spin_date"`
}

type WheelPrize struct {
	PrizeType string `json:"prize_type"`
	Amount    int    `json:"amount"`
	Name      string `json:"name"`
}

// Init opens the SQLite database, creates required tables, and inserts defaults.
func Init() error {
	db, err := sql.Open("sqlite3", dbFile)
	if err != nil {
		return err
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return err
	}

	DB = db

	if err := createTables(); err != nil {
		return err
	}

	if err := migrateTasksSchema(); err != nil {
		return err
	}

	if err := initDefaultSettings(); err != nil {
		return err
	}

	if err := initDefaultGoals(); err != nil {
		return err
	}

	if err := initDefaultTasks(); err != nil {
		return err
	}

	if err := initSafe(); err != nil {
		return err
	}

	if err := initWheelStatus(); err != nil {
		return err
	}

	return initInitialHistory()
}

func createTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS settings (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			key TEXT NOT NULL UNIQUE,
			value TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			description TEXT NOT NULL,
			amount INTEGER NOT NULL,
			created_at DATETIME NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS goals (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			cost INTEGER NOT NULL,
			is_achieved INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE TABLE IF NOT EXISTS tasks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT NOT NULL,
			reward INTEGER NOT NULL,
			status TEXT NOT NULL DEFAULT 'active'
		);`,
		`CREATE TABLE IF NOT EXISTS safe (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			balance INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE TABLE IF NOT EXISTS wheel_status (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			last_spin_date TEXT NOT NULL DEFAULT ''
		);`,
	}

	for _, query := range queries {
		if _, err := DB.Exec(query); err != nil {
			return err
		}
	}

	return nil
}

func migrateTasksSchema() error {
	rows, err := DB.Query(`PRAGMA table_info(tasks);`)
	if err != nil {
		return err
	}
	defer rows.Close()

	hasStatus := false
	hasIsCompleted := false

	for rows.Next() {
		var cid int
		var name string
		var columnType string
		var notNull int
		var defaultValue sql.NullString
		var pk int

		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return err
		}

		if name == "status" {
			hasStatus = true
		}
		if name == "is_completed" {
			hasIsCompleted = true
		}
	}

	if err := rows.Err(); err != nil {
		return err
	}

	if !hasStatus {
		if _, err := DB.Exec(`ALTER TABLE tasks ADD COLUMN status TEXT NOT NULL DEFAULT 'active';`); err != nil {
			return err
		}
	}

	if hasIsCompleted {
		_, err := DB.Exec(
			`UPDATE tasks
			 SET status = CASE WHEN is_completed = 1 THEN 'completed' ELSE 'active' END
			 WHERE status IS NULL OR status = 'active';`,
		)
		return err
	}

	return nil
}

func initDefaultSettings() error {
	defaults := map[string]string{
		"bonus_weekly":        "200",
		"retention_late_task": "50",
		"retention_bad_mark":  "100",
	}

	for key, value := range defaults {
		_, err := DB.Exec(
			`INSERT OR IGNORE INTO settings (key, value) VALUES (?, ?);`,
			key,
			value,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

func initDefaultGoals() error {
	var count int
	if err := DB.QueryRow(`SELECT COUNT(*) FROM goals;`).Scan(&count); err != nil {
		return err
	}

	if count > 0 {
		return nil
	}

	defaults := []Goal{
		{Name: "Поход в кино", Cost: 500},
		{Name: "Новая игра", Cost: 1500},
	}

	for _, goal := range defaults {
		if err := AddGoal(goal.Name, goal.Cost); err != nil {
			return err
		}
	}

	return nil
}

func initDefaultTasks() error {
	var count int
	if err := DB.QueryRow(`SELECT COUNT(*) FROM tasks;`).Scan(&count); err != nil {
		return err
	}

	if count > 0 {
		return nil
	}

	defaults := []Task{
		{Title: "Почистить зубы утром и вечером", Reward: 10},
		{Title: "Убрать постель", Reward: 10},
		{Title: "Почитать книгу 20 минут", Reward: 20},
	}

	for _, task := range defaults {
		if err := AddTask(task.Title, task.Reward); err != nil {
			return err
		}
	}

	return nil
}

func initInitialHistory() error {
	var count int
	if err := DB.QueryRow(`SELECT COUNT(*) FROM history;`).Scan(&count); err != nil {
		return err
	}

	if count > 0 {
		return nil
	}

	return AddHistory("Initial balance", 500)
}

func initSafe() error {
	_, err := DB.Exec(`INSERT INTO safe (id, balance) VALUES (1, 0) ON CONFLICT(id) DO NOTHING;`)
	return err
}

func initWheelStatus() error {
	_, err := DB.Exec(`INSERT INTO wheel_status (id, last_spin_date) VALUES (1, '') ON CONFLICT(id) DO NOTHING;`)
	return err
}

func GetSettings() ([]Setting, error) {
	rows, err := DB.Query(`SELECT key, value FROM settings ORDER BY key;`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	settings := make([]Setting, 0)
	for rows.Next() {
		var setting Setting
		if err := rows.Scan(&setting.Key, &setting.Value); err != nil {
			return nil, err
		}
		settings = append(settings, setting)
	}

	return settings, rows.Err()
}

func UpdateSettings(settings []Setting) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(
		`INSERT INTO settings (key, value)
		 VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value;`,
	)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, setting := range settings {
		if _, err := stmt.Exec(setting.Key, setting.Value); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func GetHistory() ([]HistoryRecord, error) {
	rows, err := DB.Query(
		`SELECT id, description, amount, created_at
		 FROM history
		 ORDER BY created_at DESC, id DESC;`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	history := make([]HistoryRecord, 0)
	for rows.Next() {
		var record HistoryRecord
		if err := rows.Scan(&record.ID, &record.Description, &record.Amount, &record.CreatedAt); err != nil {
			return nil, err
		}
		history = append(history, record)
	}

	return history, rows.Err()
}

func GetBalance() (int, error) {
	var balance sql.NullInt64
	if err := DB.QueryRow(`SELECT SUM(amount) FROM history;`).Scan(&balance); err != nil {
		return 0, err
	}

	if !balance.Valid {
		return 0, nil
	}

	return int(balance.Int64), nil
}

func GetGoals() ([]Goal, error) {
	rows, err := DB.Query(
		`SELECT id, name, cost, is_achieved
		 FROM goals
		 ORDER BY is_achieved ASC, cost ASC, id ASC;`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	goals := make([]Goal, 0)
	for rows.Next() {
		var goal Goal
		var isAchieved int
		if err := rows.Scan(&goal.ID, &goal.Name, &goal.Cost, &isAchieved); err != nil {
			return nil, err
		}
		goal.IsAchieved = isAchieved == 1
		goals = append(goals, goal)
	}

	return goals, rows.Err()
}

func AddGoal(name string, cost int) error {
	_, err := DB.Exec(
		`INSERT INTO goals (name, cost) VALUES (?, ?);`,
		name,
		cost,
	)
	return err
}

func BuyGoal(id int) (Goal, int, error) {
	tx, err := DB.Begin()
	if err != nil {
		return Goal{}, 0, err
	}
	defer tx.Rollback()

	var goal Goal
	var isAchieved int
	err = tx.QueryRow(
		`SELECT id, name, cost, is_achieved FROM goals WHERE id = ?;`,
		id,
	).Scan(&goal.ID, &goal.Name, &goal.Cost, &isAchieved)
	if err == sql.ErrNoRows {
		return Goal{}, 0, ErrGoalNotFound
	}
	if err != nil {
		return Goal{}, 0, err
	}

	goal.IsAchieved = isAchieved == 1
	if goal.IsAchieved {
		return Goal{}, 0, ErrGoalAlreadyBought
	}

	balance, err := getBalanceTx(tx)
	if err != nil {
		return Goal{}, 0, err
	}
	if balance < goal.Cost {
		return Goal{}, balance, ErrNotEnoughBalance
	}

	if _, err := tx.Exec(
		`INSERT INTO history (description, amount, created_at) VALUES (?, ?, ?);`,
		"Покупка: "+goal.Name,
		-goal.Cost,
		time.Now(),
	); err != nil {
		return Goal{}, 0, err
	}

	if _, err := tx.Exec(`UPDATE goals SET is_achieved = 1 WHERE id = ?;`, goal.ID); err != nil {
		return Goal{}, 0, err
	}

	if err := tx.Commit(); err != nil {
		return Goal{}, 0, err
	}

	goal.IsAchieved = true
	return goal, balance - goal.Cost, nil
}

func GetTasks() ([]Task, error) {
	rows, err := DB.Query(
		`SELECT id, title, reward, status
		 FROM tasks
		 ORDER BY
			CASE status WHEN 'active' THEN 0 WHEN 'pending' THEN 1 ELSE 2 END,
			id ASC;`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := make([]Task, 0)
	for rows.Next() {
		var task Task
		if err := rows.Scan(&task.ID, &task.Title, &task.Reward, &task.Status); err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}

	return tasks, rows.Err()
}

func AddTask(title string, reward int) error {
	_, err := DB.Exec(
		`INSERT INTO tasks (title, reward, status) VALUES (?, ?, 'active');`,
		title,
		reward,
	)
	return err
}

func CompleteTask(id int) (Task, error) {
	tx, err := DB.Begin()
	if err != nil {
		return Task{}, err
	}
	defer tx.Rollback()

	var task Task
	err = tx.QueryRow(
		`SELECT id, title, reward, status FROM tasks WHERE id = ?;`,
		id,
	).Scan(&task.ID, &task.Title, &task.Reward, &task.Status)
	if err == sql.ErrNoRows {
		return Task{}, ErrTaskNotFound
	}
	if err != nil {
		return Task{}, err
	}

	if task.Status == "completed" {
		return Task{}, ErrTaskAlreadyComplete
	}
	if task.Status != "active" {
		return Task{}, ErrTaskNotActive
	}

	if _, err := tx.Exec(`UPDATE tasks SET status = 'pending' WHERE id = ?;`, task.ID); err != nil {
		return Task{}, err
	}

	if err := tx.Commit(); err != nil {
		return Task{}, err
	}

	task.Status = "pending"
	return task, nil
}

func GetPendingTasks() ([]Task, error) {
	rows, err := DB.Query(
		`SELECT id, title, reward, status
		 FROM tasks
		 WHERE status = 'pending'
		 ORDER BY id ASC;`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := make([]Task, 0)
	for rows.Next() {
		var task Task
		if err := rows.Scan(&task.ID, &task.Title, &task.Reward, &task.Status); err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}

	return tasks, rows.Err()
}

func ReviewTask(id int, approved bool) (Task, int, error) {
	tx, err := DB.Begin()
	if err != nil {
		return Task{}, 0, err
	}
	defer tx.Rollback()

	var task Task
	err = tx.QueryRow(
		`SELECT id, title, reward, status FROM tasks WHERE id = ?;`,
		id,
	).Scan(&task.ID, &task.Title, &task.Reward, &task.Status)
	if err == sql.ErrNoRows {
		return Task{}, 0, ErrTaskNotFound
	}
	if err != nil {
		return Task{}, 0, err
	}

	if task.Status != "pending" {
		return Task{}, 0, ErrTaskNotPending
	}

	if approved {
		if _, err := tx.Exec(`UPDATE tasks SET status = 'completed' WHERE id = ?;`, task.ID); err != nil {
			return Task{}, 0, err
		}
		if _, err := tx.Exec(
			`INSERT INTO history (description, amount, created_at) VALUES (?, ?, ?);`,
			"Выполнено: "+task.Title,
			task.Reward,
			time.Now(),
		); err != nil {
			return Task{}, 0, err
		}
		task.Status = "completed"
	} else {
		if _, err := tx.Exec(`UPDATE tasks SET status = 'active' WHERE id = ?;`, task.ID); err != nil {
			return Task{}, 0, err
		}
		task.Status = "active"
	}

	if err := tx.Commit(); err != nil {
		return Task{}, 0, err
	}

	balance, err := GetBalance()
	if err != nil {
		return Task{}, 0, err
	}

	return task, balance, nil
}

func ResetTasks() error {
	_, err := DB.Exec(`UPDATE tasks SET status = 'active';`)
	return err
}

func GetSafe() (SafeInfo, error) {
	var safe SafeInfo
	if err := DB.QueryRow(`SELECT balance FROM safe WHERE id = 1;`).Scan(&safe.Balance); err != nil {
		return SafeInfo{}, err
	}

	return safe, nil
}

func DepositToSafe(amount int) (MoneyMoveResult, error) {
	tx, err := DB.Begin()
	if err != nil {
		return MoneyMoveResult{}, err
	}
	defer tx.Rollback()

	balance, err := getBalanceTx(tx)
	if err != nil {
		return MoneyMoveResult{}, err
	}
	if balance < amount {
		return MoneyMoveResult{}, ErrNotEnoughBalance
	}

	if _, err := tx.Exec(
		`INSERT INTO history (description, amount, created_at) VALUES (?, ?, ?);`,
		"Сейф: пополнение",
		-amount,
		time.Now(),
	); err != nil {
		return MoneyMoveResult{}, err
	}

	if _, err := tx.Exec(`UPDATE safe SET balance = balance + ? WHERE id = 1;`, amount); err != nil {
		return MoneyMoveResult{}, err
	}

	safeBalance, err := getSafeBalanceTx(tx)
	if err != nil {
		return MoneyMoveResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return MoneyMoveResult{}, err
	}

	return MoneyMoveResult{
		Balance:     balance - amount,
		SafeBalance: safeBalance,
		Amount:      amount,
	}, nil
}

func WithdrawFromSafe(amount int) (MoneyMoveResult, error) {
	tx, err := DB.Begin()
	if err != nil {
		return MoneyMoveResult{}, err
	}
	defer tx.Rollback()

	safeBalance, err := getSafeBalanceTx(tx)
	if err != nil {
		return MoneyMoveResult{}, err
	}
	if safeBalance < amount {
		return MoneyMoveResult{}, ErrNotEnoughSafeMoney
	}

	if _, err := tx.Exec(`UPDATE safe SET balance = balance - ? WHERE id = 1;`, amount); err != nil {
		return MoneyMoveResult{}, err
	}

	if _, err := tx.Exec(
		`INSERT INTO history (description, amount, created_at) VALUES (?, ?, ?);`,
		"Сейф: вывод",
		amount,
		time.Now(),
	); err != nil {
		return MoneyMoveResult{}, err
	}

	balance, err := getBalanceTx(tx)
	if err != nil {
		return MoneyMoveResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return MoneyMoveResult{}, err
	}

	return MoneyMoveResult{
		Balance:     balance,
		SafeBalance: safeBalance - amount,
		Amount:      amount,
	}, nil
}

func AccrueSafeInterest(percent int) (MoneyMoveResult, error) {
	tx, err := DB.Begin()
	if err != nil {
		return MoneyMoveResult{}, err
	}
	defer tx.Rollback()

	safeBalance, err := getSafeBalanceTx(tx)
	if err != nil {
		return MoneyMoveResult{}, err
	}

	interest := safeBalance * percent / 100
	if safeBalance > 0 && interest == 0 {
		interest = 1
	}

	if interest > 0 {
		if _, err := tx.Exec(`UPDATE safe SET balance = balance + ? WHERE id = 1;`, interest); err != nil {
			return MoneyMoveResult{}, err
		}
		// Interest belongs to the safe, so the history note is informational and does not change cash balance.
		if _, err := tx.Exec(
			`INSERT INTO history (description, amount, created_at) VALUES (?, ?, ?);`,
			"Проценты сейфа: +"+strconv.Itoa(interest)+" ₽",
			0,
			time.Now(),
		); err != nil {
			return MoneyMoveResult{}, err
		}
	}

	balance, err := getBalanceTx(tx)
	if err != nil {
		return MoneyMoveResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return MoneyMoveResult{}, err
	}

	return MoneyMoveResult{
		Balance:     balance,
		SafeBalance: safeBalance + interest,
		Amount:      interest,
	}, nil
}

func GetWheelStatus() (WheelStatus, error) {
	today := time.Now().Format("2006-01-02")

	var lastSpinDate string
	if err := DB.QueryRow(`SELECT last_spin_date FROM wheel_status WHERE id = 1;`).Scan(&lastSpinDate); err != nil {
		return WheelStatus{}, err
	}

	return WheelStatus{
		CanSpin:      lastSpinDate != today,
		LastSpinDate: lastSpinDate,
	}, nil
}

func SpinWheel(prize WheelPrize) (WheelStatus, int, error) {
	today := time.Now().Format("2006-01-02")

	tx, err := DB.Begin()
	if err != nil {
		return WheelStatus{}, 0, err
	}
	defer tx.Rollback()

	var lastSpinDate string
	if err := tx.QueryRow(`SELECT last_spin_date FROM wheel_status WHERE id = 1;`).Scan(&lastSpinDate); err != nil {
		return WheelStatus{}, 0, err
	}
	if lastSpinDate == today {
		return WheelStatus{
			CanSpin:      false,
			LastSpinDate: lastSpinDate,
		}, 0, ErrWheelAlreadySpun
	}

	amount := 0
	if prize.PrizeType == "money" {
		amount = prize.Amount
	}

	if _, err := tx.Exec(
		`INSERT INTO history (description, amount, created_at) VALUES (?, ?, ?);`,
		"Колесо Фортуны: "+prize.Name,
		amount,
		time.Now(),
	); err != nil {
		return WheelStatus{}, 0, err
	}

	if _, err := tx.Exec(`UPDATE wheel_status SET last_spin_date = ? WHERE id = 1;`, today); err != nil {
		return WheelStatus{}, 0, err
	}

	if err := tx.Commit(); err != nil {
		return WheelStatus{}, 0, err
	}

	return WheelStatus{
		CanSpin:      false,
		LastSpinDate: today,
	}, amount, nil
}

func getSafeBalanceTx(tx *sql.Tx) (int, error) {
	var balance int
	if err := tx.QueryRow(`SELECT balance FROM safe WHERE id = 1;`).Scan(&balance); err != nil {
		return 0, err
	}

	return balance, nil
}

func getBalanceTx(tx *sql.Tx) (int, error) {
	var balance sql.NullInt64
	if err := tx.QueryRow(`SELECT SUM(amount) FROM history;`).Scan(&balance); err != nil {
		return 0, err
	}

	if !balance.Valid {
		return 0, nil
	}

	return int(balance.Int64), nil
}

func CheckWeeklyBonus() (WeeklyBonusResult, error) {
	since := time.Now().AddDate(0, 0, -7)

	tx, err := DB.Begin()
	if err != nil {
		return WeeklyBonusResult{}, err
	}
	defer tx.Rollback()

	var retentionCount int
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM history
		 WHERE amount < 0
			AND description NOT LIKE 'Покупка:%'
			AND description NOT LIKE 'Сейф:%'
			AND created_at >= ?;`,
		since,
	).Scan(&retentionCount); err != nil {
		return WeeklyBonusResult{}, err
	}

	if retentionCount > 0 {
		if err := tx.Commit(); err != nil {
			return WeeklyBonusResult{}, err
		}

		return weeklyBonusResult("retention_found", "За последние 7 дней были удержания", false, 0)
	}

	var existingBonusCount int
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM history WHERE description = ? AND amount > 0 AND created_at >= ?;`,
		weeklyBonusDescription,
		since,
	).Scan(&existingBonusCount); err != nil {
		return WeeklyBonusResult{}, err
	}

	if existingBonusCount > 0 {
		if err := tx.Commit(); err != nil {
			return WeeklyBonusResult{}, err
		}

		return weeklyBonusResult("already_awarded", "Бонус недели уже начислен", false, 0)
	}

	amount, err := getSettingIntTx(tx, "bonus_weekly", 200)
	if err != nil {
		return WeeklyBonusResult{}, err
	}

	if _, err := tx.Exec(
		`INSERT INTO history (description, amount, created_at) VALUES (?, ?, ?);`,
		weeklyBonusDescription,
		amount,
		time.Now(),
	); err != nil {
		return WeeklyBonusResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return WeeklyBonusResult{}, err
	}

	return weeklyBonusResult("awarded", "Бонус недели начислен", true, amount)
}

func getSettingIntTx(tx *sql.Tx, key string, fallback int) (int, error) {
	var value string
	err := tx.QueryRow(`SELECT value FROM settings WHERE key = ?;`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return fallback, nil
	}
	if err != nil {
		return 0, err
	}

	number, err := strconv.Atoi(value)
	if err != nil || number <= 0 {
		return fallback, nil
	}

	return number, nil
}

func weeklyBonusResult(status string, message string, awarded bool, amount int) (WeeklyBonusResult, error) {
	balance, err := GetBalance()
	if err != nil {
		return WeeklyBonusResult{}, err
	}

	return WeeklyBonusResult{
		Status:  status,
		Message: message,
		Awarded: awarded,
		Amount:  amount,
		Balance: balance,
	}, nil
}

func AddHistory(description string, amount int) error {
	_, err := DB.Exec(
		`INSERT INTO history (description, amount, created_at) VALUES (?, ?, ?);`,
		description,
		amount,
		time.Now(),
	)
	return err
}
