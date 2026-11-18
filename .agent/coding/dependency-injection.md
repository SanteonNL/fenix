# Dependency Injection

Dependency injection reduces coupling by depending on capabilities, not construction details. Components should receive dependencies from the outside instead of creating them internally.

## Use dependency injection when dependencies vary

Prefer injection when a component depends on:

- external systems (databases, storage, APIs)
- infrastructure (S3, SFTP, email providers, queues)
- pluggable behavior (different implementations per environment or client)
- logic that may change independently of the consuming component

## Basic example

Prefer:

```go
NewInvoiceService(paymentGateway)
```

Instead of creating dependencies internally.

## Inject behavior, not construction details

A component should not know how a dependency is built, only what it can do.

Prefer:

```go
type InvoiceService struct {
    gateway PaymentGateway
}

func NewInvoiceService(gateway PaymentGateway) *InvoiceService {
    return &InvoiceService{gateway: gateway}
}
```

Instead of selecting and constructing a concrete implementation inside the service constructor.

## Depend on capabilities, not implementations

Define small interfaces at behavior boundaries.

```go
type Storage interface {
    Upload(file File) error
}
```

Then inject implementations by context (S3, SFTP, WebDAV, and so on).

## Move implementation selection into a factory

When selection depends on config, environment, or tenant, centralize it in a factory.

```go
func NewStorage(config Config) Storage {
    switch config.StorageType {
    case "s3":
        return NewS3Storage(config.S3Key)
    case "sftp":
        return NewSFTPStorage(config.SFTPHost, config.SFTPKey)
    case "webdav":
        return NewWebDAVStorage(config.WebDAVURL, config.WebDAVToken)
    default:
        panic("unknown storage type")
    }
}
```

Business logic should not care how storage was chosen.

## Keep components focused

Avoid combining unrelated responsibilities (for example, storage + encryption + processing in one component). Compose focused components instead.

## Prefer simplicity over abstraction

Do not introduce interfaces everywhere by default. Use dependency injection where it provides clear value through flexibility or separation of concerns.
