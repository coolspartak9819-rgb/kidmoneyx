package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func main() {
	if err := Init(); err != nil {
		log.Fatalf("failed to initialize database: %v", err)
	}

	router := gin.Default()

	router.GET("/api/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	router.GET("/api/settings", handleGetSettings)
	router.POST("/api/settings", handleUpdateSettings)
	router.GET("/api/history", handleGetHistory)
	router.POST("/api/history", handleAddHistory)
	router.GET("/api/balance", handleGetBalance)
	router.POST("/api/bonus/check", handleCheckWeeklyBonus)
	router.GET("/api/goals", handleGetGoals)
	router.POST("/api/goals", handleAddGoal)
	router.POST("/api/goals/buy", handleBuyGoal)
	router.GET("/api/tasks", handleGetTasks)
	router.POST("/api/tasks", handleAddTask)
	router.POST("/api/tasks/complete", handleCompleteTask)
	router.POST("/api/tasks/reset", handleResetTasks)
	router.GET("/api/parent/pending-tasks", handleGetPendingTasks)
	router.POST("/api/parent/approve-task", handleReviewTask)
	router.POST("/api/parent/accrue-interest", handleAccrueInterest)
	router.GET("/api/safe", handleGetSafe)
	router.POST("/api/safe/deposit", handleSafeDeposit)
	router.POST("/api/safe/withdraw", handleSafeWithdraw)
	router.GET("/api/wheel/status", handleWheelStatus)
	router.POST("/api/wheel/spin", handleWheelSpin)
	router.StaticFile("/", "./index.html")

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	if err := router.Run(":" + port); err != nil {
		log.Fatalf("failed to start server: %v", err)
	}
}

func handleGetSettings(c *gin.Context) {
	settings, err := GetSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load settings"})
		return
	}

	c.JSON(http.StatusOK, settings)
}

func handleUpdateSettings(c *gin.Context) {
	body, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
		return
	}

	settings, err := parseSettingsPayload(body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := UpdateSettings(settings); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update settings"})
		return
	}

	updatedSettings, err := GetSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "settings updated, but failed to reload them"})
		return
	}

	c.JSON(http.StatusOK, updatedSettings)
}

func handleGetHistory(c *gin.Context) {
	history, err := GetHistory()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load history"})
		return
	}

	c.JSON(http.StatusOK, history)
}

func handleAddHistory(c *gin.Context) {
	var request historyRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid history payload"})
		return
	}

	request.Description = strings.TrimSpace(request.Description)
	if request.Description == "" || request.Amount == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "description and non-zero amount are required"})
		return
	}

	if err := AddHistory(request.Description, request.Amount); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add history record"})
		return
	}

	balance, err := GetBalance()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "history record added, but failed to calculate balance"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"status": "ok", "balance": balance})
}

func handleGetBalance(c *gin.Context) {
	balance, err := GetBalance()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to calculate balance"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"balance": balance})
}

func handleCheckWeeklyBonus(c *gin.Context) {
	result, err := CheckWeeklyBonus()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check weekly bonus"})
		return
	}

	c.JSON(http.StatusOK, result)
}

func handleGetGoals(c *gin.Context) {
	goals, err := GetGoals()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load goals"})
		return
	}

	c.JSON(http.StatusOK, goals)
}

func handleAddGoal(c *gin.Context) {
	var request goalRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid goal payload"})
		return
	}

	request.Name = strings.TrimSpace(request.Name)
	if request.Name == "" || request.Cost <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and positive cost are required"})
		return
	}

	if err := AddGoal(request.Name, request.Cost); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add goal"})
		return
	}

	goals, err := GetGoals()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "goal added, but failed to reload goals"})
		return
	}

	c.JSON(http.StatusCreated, goals)
}

func handleBuyGoal(c *gin.Context) {
	var request buyGoalRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid buy goal payload"})
		return
	}

	goalID := request.GoalID
	if goalID == 0 {
		goalID = request.ID
	}

	if goalID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "goal id is required"})
		return
	}

	goal, balance, err := BuyGoal(goalID)
	if err != nil {
		switch {
		case errors.Is(err, ErrGoalNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "goal not found"})
		case errors.Is(err, ErrGoalAlreadyBought):
			c.JSON(http.StatusConflict, gin.H{"error": "goal already bought"})
		case errors.Is(err, ErrNotEnoughBalance):
			c.JSON(http.StatusBadRequest, gin.H{"error": "Недостаточно средств", "balance": balance})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to buy goal"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok", "goal": goal, "balance": balance})
}

func handleGetTasks(c *gin.Context) {
	tasks, err := GetTasks()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load tasks"})
		return
	}

	c.JSON(http.StatusOK, tasks)
}

func handleAddTask(c *gin.Context) {
	var request taskRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task payload"})
		return
	}

	request.Title = strings.TrimSpace(request.Title)
	if request.Title == "" || request.Reward <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title and positive reward are required"})
		return
	}

	if err := AddTask(request.Title, request.Reward); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add task"})
		return
	}

	tasks, err := GetTasks()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "task added, but failed to reload tasks"})
		return
	}

	c.JSON(http.StatusCreated, tasks)
}

func handleCompleteTask(c *gin.Context) {
	var request completeTaskRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid complete task payload"})
		return
	}

	taskID := request.TaskID
	if taskID == 0 {
		taskID = request.ID
	}

	if taskID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task id is required"})
		return
	}

	task, err := CompleteTask(taskID)
	if err != nil {
		switch {
		case errors.Is(err, ErrTaskNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		case errors.Is(err, ErrTaskAlreadyComplete):
			c.JSON(http.StatusConflict, gin.H{"error": "task already completed"})
		case errors.Is(err, ErrTaskNotActive):
			c.JSON(http.StatusConflict, gin.H{"error": "task is already waiting for review"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to complete task"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "pending", "task": task})
}

func handleResetTasks(c *gin.Context) {
	if err := ResetTasks(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reset tasks"})
		return
	}

	tasks, err := GetTasks()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tasks reset, but failed to reload tasks"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok", "tasks": tasks})
}

func handleGetPendingTasks(c *gin.Context) {
	tasks, err := GetPendingTasks()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load pending tasks"})
		return
	}

	c.JSON(http.StatusOK, tasks)
}

func handleReviewTask(c *gin.Context) {
	var request reviewTaskRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid review task payload"})
		return
	}

	taskID := request.TaskID
	if taskID == 0 {
		taskID = request.ID
	}

	if taskID <= 0 || (request.Decision != "approved" && request.Decision != "rejected") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task_id and decision are required"})
		return
	}

	task, balance, err := ReviewTask(taskID, request.Decision == "approved")
	if err != nil {
		switch {
		case errors.Is(err, ErrTaskNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		case errors.Is(err, ErrTaskNotPending):
			c.JSON(http.StatusConflict, gin.H{"error": "task is not pending"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to review task"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok", "task": task, "balance": balance})
}

func handleGetSafe(c *gin.Context) {
	safe, err := GetSafe()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load safe"})
		return
	}

	c.JSON(http.StatusOK, safe)
}

func handleSafeDeposit(c *gin.Context) {
	var request amountRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.Amount <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "positive amount is required"})
		return
	}

	result, err := DepositToSafe(request.Amount)
	if err != nil {
		if errors.Is(err, ErrNotEnoughBalance) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Недостаточно средств"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to deposit to safe"})
		return
	}

	c.JSON(http.StatusOK, result)
}

func handleSafeWithdraw(c *gin.Context) {
	var request amountRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.Amount <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "positive amount is required"})
		return
	}

	result, err := WithdrawFromSafe(request.Amount)
	if err != nil {
		if errors.Is(err, ErrNotEnoughSafeMoney) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Недостаточно денег в сейфе"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to withdraw from safe"})
		return
	}

	c.JSON(http.StatusOK, result)
}

func handleAccrueInterest(c *gin.Context) {
	result, err := AccrueSafeInterest(5)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to accrue interest"})
		return
	}

	c.JSON(http.StatusOK, result)
}

func handleWheelStatus(c *gin.Context) {
	status, err := GetWheelStatus()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load wheel status"})
		return
	}

	c.JSON(http.StatusOK, status)
}

func handleWheelSpin(c *gin.Context) {
	var request wheelPrizeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid wheel prize payload"})
		return
	}

	request.Name = strings.TrimSpace(request.Name)
	if request.Name == "" || (request.PrizeType != "money" && request.PrizeType != "gift") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "valid prize_type and name are required"})
		return
	}
	if request.PrizeType == "money" && request.Amount <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "money prize requires positive amount"})
		return
	}

	status, amount, err := SpinWheel(WheelPrize{
		PrizeType: request.PrizeType,
		Amount:    request.Amount,
		Name:      request.Name,
	})
	if err != nil {
		if errors.Is(err, ErrWheelAlreadySpun) {
			c.JSON(http.StatusConflict, gin.H{"error": "wheel already spun today", "can_spin": false})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to spin wheel"})
		return
	}

	balance, err := GetBalance()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "wheel spun, but failed to calculate balance"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   "ok",
		"can_spin": status.CanSpin,
		"amount":   amount,
		"balance":  balance,
	})
}

type historyRequest struct {
	Description string `json:"description"`
	Amount      int    `json:"amount"`
}

type goalRequest struct {
	Name string `json:"name"`
	Cost int    `json:"cost"`
}

type buyGoalRequest struct {
	GoalID int `json:"goal_id"`
	ID     int `json:"id"`
}

type taskRequest struct {
	Title  string `json:"title"`
	Reward int    `json:"reward"`
}

type completeTaskRequest struct {
	TaskID int `json:"task_id"`
	ID     int `json:"id"`
}

type reviewTaskRequest struct {
	TaskID   int    `json:"task_id"`
	ID       int    `json:"id"`
	Decision string `json:"decision"`
}

type amountRequest struct {
	Amount int `json:"amount"`
}

type wheelPrizeRequest struct {
	PrizeType string `json:"prize_type"`
	Amount    int    `json:"amount"`
	Name      string `json:"name"`
}

func parseSettingsPayload(body []byte) ([]Setting, error) {
	var settings []Setting
	if err := json.Unmarshal(body, &settings); err == nil {
		return validateSettings(settings)
	}

	var values map[string]string
	if err := json.Unmarshal(body, &values); err == nil {
		settings = make([]Setting, 0, len(values))
		for key, value := range values {
			settings = append(settings, Setting{Key: key, Value: value})
		}

		return validateSettings(settings)
	}

	var numericValues map[string]int
	if err := json.Unmarshal(body, &numericValues); err == nil {
		settings = make([]Setting, 0, len(numericValues))
		for key, value := range numericValues {
			settings = append(settings, Setting{Key: key, Value: strconv.Itoa(value)})
		}

		return validateSettings(settings)
	}

	return nil, errInvalidSettingsPayload
}

func validateSettings(settings []Setting) ([]Setting, error) {
	if len(settings) == 0 {
		return nil, errEmptySettingsPayload
	}

	for i := range settings {
		settings[i].Key = strings.TrimSpace(settings[i].Key)
		settings[i].Value = strings.TrimSpace(settings[i].Value)

		if settings[i].Key == "" || settings[i].Value == "" {
			return nil, errInvalidSettingsPayload
		}
	}

	return settings, nil
}

type apiError string

func (e apiError) Error() string {
	return string(e)
}

const (
	errInvalidSettingsPayload apiError = "settings payload must be an array of {key, value} objects or a key-value object"
	errEmptySettingsPayload   apiError = "settings payload must contain at least one setting"
)
