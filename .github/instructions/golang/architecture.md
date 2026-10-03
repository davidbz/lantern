---
applyTo: "**/*.go"
---

## Go Architecture Conventions

### Data-Oriented Design - Separate Logic and Data

We follow data-oriented programming principles. Keep data structures simple and separate from behavior. This makes code more maintainable, testable, and easier to reason about.

**Core Principles:**
- Data structures should be simple, focused, and contain only data
- Logic operates on data through functions, not methods on data structures
- Prefer pure functions that transform data over stateful objects
- Design around data flow, not object hierarchies
- Avoid traditional OOP patterns that mix state and behavior

**Do:**
```go
// models/user.go - Pure data structures
package models

type User struct {
	ID        string
	Email     string
	Role      string
	IsActive  bool
	CreatedAt time.Time
}

type RolePermissions struct {
	CanEdit   bool
	CanDelete bool
	CanInvite bool
}

// Static data defining the permission model.
// A pure function instead of a package-level map: no globals (gochecknoglobals), nothing to mutate.
func PermissionsFor(role string) RolePermissions {
	switch role {
	case "admin":
		return RolePermissions{CanEdit: true, CanDelete: true, CanInvite: true}
	case "editor":
		return RolePermissions{CanEdit: true, CanDelete: false, CanInvite: false}
	default:
		return RolePermissions{CanEdit: false, CanDelete: false, CanInvite: false}
	}
}

// services/user.go - Logic operates on data
package services

// GOOD - Struct contains only business logic dependencies and static config
type UserService struct {
	repo   UserRepository  // Business logic interface
	auth   AuthService     // Business logic interface
	config ServiceConfig   // Static configuration
}

func NewUserService(repo UserRepository, auth AuthService, config ServiceConfig) *UserService {
	return &UserService{
		repo:   repo,
		auth:   auth,
		config: config,
	}
}

// Pure function operating on user data
func (s *UserService) CreateUser(ctx context.Context, user *models.User) error {
	if err := ValidateUser(user); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	if err := s.repo.Save(ctx, user); err != nil {
		return fmt.Errorf("failed to save user: %w", err)
	}

	return nil
}

// Pure function that transforms user data
func ValidateUser(user *models.User) error {
	if user == nil {
		return errors.New("user is nil")
	}
	if user.Email == "" {
		return errors.New("email is required")
	}
	if !isValidEmail(user.Email) {
		return errors.New("invalid email format")
	}
	return nil
}
```

**Don't:**
```go
// BAD - Mixing data, state, and dependencies
type User struct {
	ID       string
	Email    string
	Role     string
	IsActive bool

	// BAD - embedding technical dependencies in data structure
	db     *sql.DB
	cache  Cache
	logger Logger
}

// BAD - Data structure with complex behavior and hidden dependencies
func (u *User) Save() error {
	// Hidden dependency on database
	return u.db.Insert(u)
}

func (u *User) HasPermission(action string) bool {
	// Complex logic embedded in data structure
	if u.Role == "admin" {
		return true
	}
	// More logic...
	return false
}

// BAD - Service with mutable state
type UserService struct {
	repo        UserRepository
	currentUser *User           // State - DON'T store request data
	lastError   error          // State - DON'T accumulate state
	cache       map[string]any // Mutable state - DON'T
	requestID   string         // Request data - DON'T (use context)
}

// BAD - Stateful object that changes during use
func (s *UserService) SetCurrentUser(user *User) {
	s.currentUser = user // Mutating state - BAD!
}

func (s *UserService) Process() error {
	// Operating on stored state instead of parameters
	return s.repo.Save(s.currentUser)
}
```

**Quick Check for Good Struct Design:**

Look at your struct members. If all members are either:
1. **Business logic interfaces** (Repository, Service, etc.), OR
2. **Static configuration** structs loaded at startup

...then you're on the right track!

If you see any of these, refactor:
- ❌ Data that changes during request processing
- ❌ Caches, counters, or accumulated state
- ❌ Request-scoped data (use context instead)
- ❌ Embedded technical clients (db connections, HTTP clients)

**Key Points:**
- Keep data structures in `models/` or `domain/` packages with only fields
- Business logic goes in `services/` as functions operating on data
- Services contain dependencies (interfaces) and static config only - zero mutable state
- Think about data transformation pipelines, not object interactions
- Pass data as function parameters, don't store it in structs

### Always Return Errors - Never Best Effort

When an error occurs, always return it immediately. Do not try to continue with "best effort" approaches or silently log and ignore errors. This prevents cascading failures and makes debugging easier.

**Do:**
```go
func CreateUserAccount(ctx context.Context, user *User) error {
	if err := validateUser(user); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	if err := db.SaveUser(ctx, user); err != nil {
		return fmt.Errorf("failed to save user: %w", err)
	}

	if err := emailService.SendWelcome(ctx, user.Email); err != nil {
		return fmt.Errorf("failed to send welcome email: %w", err)
	}

	return nil
}
```

**Don't:**
```go
func CreateUserAccount(ctx context.Context, user *User) error {
	if err := validateUser(user); err != nil {
		log.Error("validation failed", err)
		// DON'T: Continuing despite validation failure
	}

	if err := db.SaveUser(ctx, user); err != nil {
		log.Error("failed to save user", err)
		// DON'T: Pretending everything is fine
		return nil
	}

	if err := emailService.SendWelcome(ctx, user.Email); err != nil {
		log.Error("failed to send email", err)
		// DON'T: Swallowing error and returning success
	}

	return nil // Lying about success!
}
```

### Don't Panic!
Don’t use panic for normal error handling. Use error and multiple return values.
panic should be reserved for truly exceptional cases, or conditions that should be impossible. Things like: reaching the end of an infinite loop, or attempting to divide by zero.

When deciding “Should I return an error, or panic?” you should almost always prefer returning an error.

### Context Usage Guidelines

Context should only carry request-scoped primitive data and must never be stored in structs. Every context must have a trace ID for observability.

**What to Store in Context:**
- Trace ID (required for all contexts)
- User ID / Username
- Tenant ID
- IP Address
- Request ID
- Authentication tokens

**Never Store in Context:**
- Large objects or structs
- Database connections
- Business logic data
- Application state
- Configuration

Context keys must be unexported, typed keys (never plain strings - see staticcheck SA1029 / revive
`context-keys-type`). Expose small typed accessors from the package that owns the key:

**Do:**
```go
// internal/requestctx/requestctx.go
package requestctx

type key int

const (
	traceIDKey key = iota
	userIDKey
	tenantIDKey
	ipAddressKey
)

func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey, traceID)
}

func TraceID(ctx context.Context) string {
	traceID, _ := ctx.Value(traceIDKey).(string)
	return traceID
}

// WithUserID, WithTenantID, WithIPAddress follow the same pattern.

// Creating context with request-scoped data
func HandleRequest(w http.ResponseWriter, r *http.Request) {
	// Always add trace ID first
	ctx := requestctx.WithTraceID(r.Context(), uuid.NewString())

	// Add request-scoped primitives
	ctx = requestctx.WithUserID(ctx, getUserID(r))
	ctx = requestctx.WithTenantID(ctx, getTenantID(r))
	ctx = requestctx.WithIPAddress(ctx, r.RemoteAddr)

	processRequest(ctx, w, r)
}

// For async operations, detach context to prevent timeout
func ProcessAsync(ctx context.Context, data *Data) {
	// Detach context - copy values but remove deadline
	detachedCtx := context.WithoutCancel(ctx)

	go func() {
		// Use detached context so async work won't timeout.
		// The logger from context already carries the trace ID.
		logger := logging.FromContext(detachedCtx)
		logger.InfoContext(detachedCtx, "processing async task")

		if err := performBackgroundWork(detachedCtx, data); err != nil {
			// The goroutine has no caller to return to: logging is the only option here.
			logger.ErrorContext(detachedCtx, "async task failed", "error", err)
		}
	}()
}
```

**Don't:**
```go
// BAD - storing context in struct
type UserService struct {
	ctx  context.Context  // NEVER do this!
	repo UserRepository
}

// BAD - storing complex objects in context
type RequestData struct {
	User    *User
	Config  *Config
	Cache   *Cache
}
ctx = context.WithValue(ctx, "request_data", requestData) // Don't!

// BAD - using parent context in goroutine (will timeout)
func ProcessAsync(ctx context.Context, data *Data) {
	go func() {
		// Using parent context - will timeout when parent request ends!
		if err := performBackgroundWork(ctx, data); err != nil {
			// This might fail due to context deadline exceeded
		}
	}()
}

// BAD - no trace ID
func HandleRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// Missing trace ID - can't trace this request through the system
	ctx = context.WithValue(ctx, "user_id", getUserID(r))
	processRequest(ctx, r)
}
```

**Key Points:**
- Every context MUST have a trace ID as the first value added
- Only store primitive types: strings, ints, bools
- Never store context in struct fields - always pass as function parameter
- Use `context.WithoutCancel()` when spawning goroutines for async work
- Context values are for request-scoped data only, not application state

### Logger Should Always Come from Context

Create loggers from context to ensure proper request tracing and structured logging throughout the application lifecycle.

**Do:**
```go
func ProcessOrder(ctx context.Context, order *Order) error {
	logger := logging.FromContext(ctx)

	logger.InfoContext(ctx, "processing order", "order_id", order.ID, "user_id", order.UserID)

	if err := validateOrder(order); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	if err := chargePayment(ctx, order); err != nil {
		return fmt.Errorf("payment failed for order %s: %w", order.ID, err)
	}

	logger.InfoContext(ctx, "order processed successfully", "order_id", order.ID)
	return nil
}
```

**Don't:**
```go
// Bad - using global logger
var globalLogger = log.New()

func ProcessOrder(ctx context.Context, order *Order) error {
	globalLogger.Info("processing order") // No context, no structured fields

	if err := validateOrder(order); err != nil {
		globalLogger.Error(err.Error()) // Lost all context
		return fmt.Errorf("validation failed: %w", err)
	}

	return nil
}
```

**Key Points:**
- Logger must be extracted from context using `logging.FromContext(ctx)`
- Use the `*Context` methods (`InfoContext`, `ErrorContext`, ...) whenever a ctx is in scope (enforced by sloglint)
- Log messages are static strings; variable data goes in snake_case key-value pairs (enforced by sloglint)
- Don't log an error *and* return it - return it wrapped, and let the top-level handler log it once
- Use structured logging with key-value pairs
- Include relevant identifiers (IDs, user info, etc.) in log fields
- Never use global loggers or create loggers outside of context

### Package Organization

Organize code using `cmd/` for entry points and `internal/` for application code:

```
project/
├── cmd/
│   ├── api/
│   │   └── main.go          # API server entry point
│   └── worker/
│       └── main.go          # Background worker entry point
├── internal/
│   ├── domain/              # Business entities and core logic
│   ├── handlers/            # HTTP/gRPC handlers
│   ├── repository/          # Data access layer
│   ├── services/            # Business services
│   └── config/              # Configuration
└── go.mod
```

**Key Points:**
- `cmd/` - Contains `main.go` files for each binary
- `internal/` - All application code (cannot be imported externally)
- No `pkg/` directory - this template is for applications, not libraries

### Avoid Technology Names in Main Business Flow

If you spot technology names or specific component names in your main business flow, it's not the way to go. Business logic should be abstracted and not coupled to specific technologies.

**Do:**
```go
// Main business flow - no technology names visible
func ProcessUserRegistration(ctx context.Context, userData UserRegistrationData) error {
	// Validate user data
	if err := validateRegistrationData(userData); err != nil {
		return fmt.Errorf("invalid registration data: %w", err)
	}

	// Create user account
	user, err := createUserAccount(ctx, userData)
	if err != nil {
		return fmt.Errorf("failed to create user account: %w", err)
	}

	// Send welcome notification
	if err := sendWelcomeMessage(ctx, user); err != nil {
		return fmt.Errorf("failed to send welcome message: %w", err)
	}

	// Provision access
	if err := provisionUserAccess(ctx, user); err != nil {
		return fmt.Errorf("failed to provision access: %w", err)
	}

	return nil
}

// Technology-specific implementations are abstracted away
type NotificationService interface {
	SendWelcome(ctx context.Context, user *User) error
}

type AccessProvisionService interface {
	ProvisionAccess(ctx context.Context, user *User) error
}
```

**Don't:**
```go
// Bad - technology names leak into business logic
func ProcessUserRegistration(ctx context.Context, userData UserRegistrationData) error {
	// Validate user data
	if err := validateRegistrationData(userData); err != nil {
		return err
	}

	// Create user account
	user, err := createUserAccount(ctx, userData)
	if err != nil {
		return err
	}

	// Send email via SendGrid - technology name in business flow!
	if err := sendGridClient.SendWelcomeEmail(ctx, user.Email); err != nil {
		log.Error("SendGrid failed", "error", err)
	}

	// Provision in Active Directory - technology name in business flow!
	if err := activeDirectoryClient.CreateUser(ctx, user); err != nil {
		return fmt.Errorf("AD provisioning failed: %w", err)
	}

	// Update Salesforce - technology name in business flow!
	return salesforceClient.CreateContact(ctx, user)
}
```

### Don't Mix Standard Logic and Technical Integrations

Separate core business logic from technical integrations like SailPoint. Use the observer pattern to keep these concerns decoupled.

**Do:**
```go
// domain/user_service.go
package domain

import (
	"context"
	"fmt"
)

// EventUserCreated is published after a user is persisted.
const EventUserCreated = "user.created"

// UserService handles core user business logic
type UserService struct {
	repo           UserRepository
	eventPublisher EventPublisher
}

func NewUserService(repo UserRepository, publisher EventPublisher) *UserService {
	return &UserService{
		repo:           repo,
		eventPublisher: publisher,
	}
}

func (s *UserService) CreateUser(ctx context.Context, user *User) error {
	// Core business logic
	if err := validateUser(user); err != nil {
		return fmt.Errorf("invalid user: %w", err)
	}

	// Save to database
	if err := s.repo.Save(ctx, user); err != nil {
		return fmt.Errorf("failed to save user: %w", err)
	}

	// Publish event for observers (like SailPoint) to react to
	if err := s.eventPublisher.Publish(ctx, EventUserCreated, user); err != nil {
		return fmt.Errorf("failed to publish %s: %w", EventUserCreated, err)
	}

	return nil
}

// integrations/sailpoint/observer.go
package sailpoint

import (
	"context"
	"fmt"

	"github.com/davidbz/golang-template/internal/domain"
)

// SailPointObserver implements the Observer interface
type SailPointObserver struct {
	client SailPointClient
}

func (o *SailPointObserver) OnEvent(ctx context.Context, eventType string, data any) error {
	if eventType != domain.EventUserCreated {
		return nil // not interested in other events
	}

	user, ok := data.(*domain.User)
	if !ok {
		return fmt.Errorf("unexpected payload %T for %s", data, eventType)
	}

	if err := o.client.SyncUser(ctx, user); err != nil {
		return fmt.Errorf("failed to sync user to SailPoint: %w", err)
	}

	return nil
}
```

### Everything Goes Through IoC (Dependency Injection) Using Dig

We use [go-dig](https://github.com/uber-go/dig) for dependency injection in Go projects.

**Do:**
```go
import (
	"go.uber.org/dig"
)

// Services with constructor-based dependency injection
type UserController struct {
	userService UserService
	authService AuthService
}

func NewUserController(userService UserService, authService AuthService) *UserController {
	return &UserController{
		userService: userService,
		authService: authService,
	}
}

// In main.go or setup.go
func BuildContainer() (*dig.Container, error) {
	container := dig.New()

	// Register dependencies - Provide fails on duplicate or invalid constructors, so check every call.
	constructors := []any{
		initDatabase,
		NewEventBus,
		NewSQLUserRepository,
		NewAuthService,
		NewUserService,
		NewUserController,
	}
	for _, constructor := range constructors {
		if err := container.Provide(constructor); err != nil {
			return nil, fmt.Errorf("failed to register constructor: %w", err)
		}
	}

	return container, nil
}

// cmd/<app>/main.go - the composition root is the only place allowed to give up.
func main() {
	container, err := BuildContainer()
	if err != nil {
		log.Fatalf("failed to build container: %v", err)
	}

	// Invoke the application entry point
	err = container.Invoke(func(controller *UserController) error {
		return NewServer(controller).Start()
	})
	if err != nil {
		log.Fatalf("application stopped: %v", err)
	}
}
```

### Configuration with godotenv and Dependency Injection

Use `godotenv` to load environment files and `caarlos0/env` to parse configuration. Load once at startup and distribute via DI.

**Do:**
```go
// config/config.go
type Config struct {
	Server ServerConfig
	Auth   AuthConfig
}

type ServerConfig struct {
	Port int `env:"SERVER_PORT" envDefault:"8080"`
}

type AuthConfig struct {
	ClientID string `env:"AUTH_CLIENT_ID"`
	Secret   string `env:"AUTH_CLIENT_SECRET"`
}

// DepConfig for dependency injection
type DepConfig struct {
	dig.Out
	*ServerConfig
	*AuthConfig
}

func Load() (*Config, error) {
	for _, file := range []string{".env", ".env.defaults", ".env.secrets"} {
		// Env files are optional, but a file that exists and can't be read is an error.
		if err := godotenv.Load(file); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("failed to load %s: %w", file, err)
		}
	}

	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config from environment: %w", err)
	}

	return &cfg, nil
}

func ParseDependenciesConfig(cfg *Config) DepConfig {
	return DepConfig{
		Out:          dig.Out{},
		ServerConfig: &cfg.Server,
		AuthConfig:   &cfg.Auth,
	}
}

// main.go (check each Provide error as shown above)
container.Provide(config.Load)
container.Provide(config.ParseDependenciesConfig)
container.Provide(func(cfg *config.AuthConfig) *auth.Service {
	return auth.NewService(cfg)
})
```

**Don't:**
```go
// Bad - reading env vars directly in business logic
host := os.Getenv("HOST")

// Bad - global config accessed everywhere
var GlobalConfig *Config
```
