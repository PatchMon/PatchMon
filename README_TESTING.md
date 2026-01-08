# Testing Guide for SSH Terminal Feature

This document describes the test suite for the SSH Terminal feature in PatchMon.

## Test Structure

### Backend Tests (`backend/__tests__/`)

#### Unit Tests
- **`services/sshTerminalWs.test.js`**: Comprehensive unit tests for the SSH Terminal WebSocket service
  - Path parsing and validation
  - Authentication and authorization
  - Host validation
  - WebSocket message handling (connect, input, resize, disconnect)
  - Error handling
  - Token extraction from query params and headers

#### Integration Tests
- **`integration/sshTerminal.integration.test.js`**: End-to-end integration tests
  - Full SSH connection flow
  - Concurrent connections
  - Error scenarios
  - Security validations
  - Performance tests

### Frontend Tests (`frontend/src/__tests__/`)

#### Component Tests
- **`components/SshTerminal.test.jsx`**: React component tests for the SSH Terminal UI
  - Component rendering (modal and embedded modes)
  - Authentication method switching (password vs SSH key)
  - Form validation
  - WebSocket connection handling
  - Token validation
  - Connection state management
  - Disconnect functionality
  - Sidebar management
  - Install command display

## Running Tests

### Backend Tests

```bash
cd backend

# Run all tests
npm test

# Run tests in watch mode
npm run test:watch

# Run tests with coverage
npm run test:coverage
```

### Frontend Tests

```bash
cd frontend

# Run all tests
npm test

# Run tests with UI
npm run test:ui

# Run tests with coverage
npm run test:coverage
```

## Test Coverage

### Backend Coverage

The backend tests cover:
- ✅ WebSocket upgrade handling
- ✅ JWT token validation
- ✅ Session management
- ✅ Host lookup and validation
- ✅ SSH client connection (password and key auth)
- ✅ Message routing (connect, input, resize, disconnect)
- ✅ Error handling and cleanup
- ✅ Security validations

### Frontend Coverage

The frontend tests cover:
- ✅ Component rendering in both modal and embedded modes
- ✅ Authentication method selection
- ✅ Form validation
- ✅ WebSocket connection lifecycle
- ✅ Token validation
- ✅ Connection state management
- ✅ User interactions (connect, disconnect)
- ✅ Error display
- ✅ Sidebar management

## Test Dependencies

### Backend
- **Jest**: Testing framework
- **ssh2**: Mocked for testing SSH connections
- **ws**: Mocked for testing WebSocket connections

### Frontend
- **Vitest**: Testing framework (works with Vite)
- **@testing-library/react**: React component testing utilities
- **@testing-library/jest-dom**: DOM matchers
- **jsdom**: DOM environment for tests

## Writing New Tests

### Backend Test Example

```javascript
describe("Feature Name", () => {
  beforeEach(() => {
    // Setup mocks and test data
  });

  it("should do something", async () => {
    // Arrange
    // Act
    // Assert
  });
});
```

### Frontend Test Example

```javascript
describe("Component Name", () => {
  it("should render correctly", () => {
    render(<Component />);
    expect(screen.getByText("Expected Text")).toBeInTheDocument();
  });
});
```

## Mocking

### Backend Mocks
- Prisma client is mocked to avoid database dependencies
- SSH2 Client is mocked for SSH connection tests
- WebSocket Server is mocked for WebSocket tests
- Session manager is mocked for authentication tests

### Frontend Mocks
- WebSocket is mocked with a test implementation
- xterm Terminal is mocked
- API calls are mocked
- localStorage is mocked

## Integration Test Setup

Integration tests require:
1. Test database with seeded data
2. Mock SSH server or test SSH server
3. WebSocket server running
4. Test user and host records

To run integration tests, set up a test environment:

```bash
# Set test environment variables
export NODE_ENV=test
export DATABASE_URL="postgresql://user:pass@localhost:5432/patchmon_test"
export JWT_SECRET="test-secret"

# Run integration tests
npm test -- __tests__/integration
```

## Continuous Integration

Tests are designed to run in CI/CD pipelines. Make sure to:
1. Set up test database
2. Install dependencies
3. Run tests before deployment

## Troubleshooting

### Backend Tests Failing

1. **JWT_SECRET not set**: Ensure test setup file sets JWT_SECRET
2. **Prisma client issues**: Check that Prisma is properly mocked
3. **WebSocket upgrade errors**: Verify WebSocket.Server is properly mocked

### Frontend Tests Failing

1. **WebSocket not defined**: Check test setup file includes WebSocket mock
2. **xterm import errors**: Verify xterm is properly mocked
3. **React Query errors**: Ensure QueryClientProvider wraps components

## Coverage Goals

- **Backend**: Aim for >80% coverage on SSH terminal service
- **Frontend**: Aim for >80% coverage on SSH terminal component
- **Integration**: Cover all critical user flows

## Notes

- Tests use mocks to avoid external dependencies
- Integration tests may require additional setup
- Some tests may need adjustment based on actual implementation details
- WebSocket and SSH connection tests require careful mocking
