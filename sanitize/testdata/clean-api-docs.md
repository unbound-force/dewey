# API Documentation

## Authentication

All API requests require a Bearer token in the Authorization header:

```
Authorization: Bearer <your-token>
```

Tokens are issued via the `/auth/token` endpoint using OAuth 2.0 client credentials flow.

## Endpoints

### GET /api/v1/users

Returns a paginated list of users.

**Query Parameters:**
- `offset` (int, default: 0) — Starting position
- `limit` (int, default: 20, max: 100) — Number of results

**Response:**
```json
{
  "users": [
    {"id": "u123", "name": "Alice", "email": "alice@example.com"}
  ],
  "total": 42,
  "offset": 0,
  "limit": 20
}
```

### POST /api/v1/users

Creates a new user account.

**Request Body:**
```json
{
  "name": "Bob",
  "email": "bob@example.com",
  "role": "member"
}
```

### DELETE /api/v1/users/:id

Deletes a user by ID. Requires admin role.

## Rate Limiting

API requests are rate-limited to 100 requests per minute per API key.
Exceeding the limit returns HTTP 429 with a `Retry-After` header.

## Error Handling

All errors follow the standard format:

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Email is required",
    "details": []
  }
}
```
