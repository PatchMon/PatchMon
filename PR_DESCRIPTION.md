# SSH Terminal Access Feature + Comprehensive Test Suite

## Summary

This PR implements SSH terminal access directly from the PatchMon UI, allowing users to SSH into monitored hosts without leaving the dashboard. The feature includes both password and SSH key authentication, with a full test suite covering all functionality.

**Closes #307**

## Features

### SSH Terminal Access
- ✅ WebSocket-based SSH terminal connection
- ✅ Support for password authentication
- ✅ Support for SSH key authentication (including encrypted keys with passphrase)
- ✅ Terminal resize handling
- ✅ Idle timeout (15 minutes with 1-minute warning)
- ✅ Automatic reconnection on connection loss
- ✅ Error handling and user feedback
- ✅ Modal and embedded display modes
- ✅ Integration with HostDetail page
- ✅ Agent installation command helper

### Security
- ✅ JWT token-based authentication
- ✅ Session validation
- ✅ Credentials never stored (only used in-memory during connection)
- ✅ Secure credential cleanup on disconnect

## Test Coverage

### Backend Tests (37 tests, all passing ✅)
- Path parsing and validation
- JWT token authentication
- Session management
- Host validation
- WebSocket message handling structure
- Error handling
- Token extraction from query params and headers

### Frontend Tests
- Component rendering (modal and embedded modes)
- Authentication method switching
- Form validation
- WebSocket connection handling
- Token validation
- Connection state management
- Disconnect functionality

### Integration Tests
- Test structure for end-to-end flows
- Ready for expansion with real test infrastructure

## Changes

### Backend
- **New Service**: `backend/src/services/sshTerminalWs.js` - SSH Terminal WebSocket handler
- **Updated**: `backend/src/services/agentWs.js` - Added SSH terminal route handling
- **Updated**: `backend/src/routes/hostRoutes.js` - Added SSH terminal endpoint
- **Dependencies**: Added `ssh2` package

### Frontend
- **New Component**: `frontend/src/components/SshTerminal.jsx` - SSH Terminal UI component
- **Updated**: `frontend/src/pages/HostDetail.jsx` - Integrated SSH terminal button
- **Updated**: `frontend/src/components/Layout.jsx` - Sidebar management
- **New Context**: `frontend/src/contexts/SidebarContext.jsx` - Sidebar state management
- **Updated**: `frontend/vite.config.js` - WebSocket proxy configuration
- **Dependencies**: Added `xterm` and `xterm-addon-fit` packages

### Tests
- **Backend**: Jest test suite with 37 passing tests
- **Frontend**: Vitest test suite for component testing
- **Documentation**: Comprehensive testing guide (`README_TESTING.md`)

## Testing

### Run Backend Tests
```bash
cd backend
npm install
npm test
```

### Run Frontend Tests
```bash
cd frontend
npm install
npm test
```

### Manual Testing
1. Navigate to a host detail page
2. Click "SSH Terminal" button
3. Enter SSH credentials (password or SSH key)
4. Verify terminal connection works
5. Test terminal resize
6. Test idle timeout (15 minutes)
7. Test reconnection after network interruption

## Screenshots

### SSH Terminal Modal View
![SSH Terminal Modal](https://raw.githubusercontent.com/slibonati/PatchMon/feature/ssh-terminal-access/docs/images/ssh-terminal/Screenshot%20From%202025-12-06%2012-12-19.png)

### Active SSH Connection
![SSH Terminal Connected](https://raw.githubusercontent.com/slibonati/PatchMon/feature/ssh-terminal-access/docs/images/ssh-terminal/Screenshot%20From%202025-12-06%2012-12-31.png)

### Embedded Mode on HostDetail Page
![SSH Terminal Embedded](https://raw.githubusercontent.com/slibonati/PatchMon/feature/ssh-terminal-access/docs/images/ssh-terminal/Screenshot%20From%202025-12-06%2012-12-55.png)

### Authentication Methods
![SSH Terminal Auth Methods](https://raw.githubusercontent.com/slibonati/PatchMon/feature/ssh-terminal-access/docs/images/ssh-terminal/Screenshot%20From%202025-12-06%2012-13-14.png)

## Technical Details

### WebSocket Endpoint
- Path: `/api/v1/ssh-terminal/:hostId`
- Authentication: JWT token via query parameter or Authorization header
- Protocol: WebSocket upgrade from HTTP

### SSH Connection Flow
1. User clicks "SSH Terminal" button
2. Frontend opens WebSocket connection with JWT token
3. Backend validates token and host access
4. User provides SSH credentials (password or key)
5. Backend establishes SSH connection using `ssh2` library
6. Terminal I/O is proxied through WebSocket
7. Connection cleaned up on disconnect

### Security Considerations
- SSH credentials are never stored in database
- Credentials only exist in memory during active connection
- Credentials are cleared immediately on disconnect
- JWT token validated on every WebSocket connection
- Session activity tracked for security

## Documentation

- **Testing Guide**: See `README_TESTING.md` for detailed testing documentation
- **Code Comments**: Inline documentation in service and component files

## Checklist

- [x] Feature implementation complete
- [x] Backend tests written and passing (37/37)
- [x] Frontend tests written
- [x] Documentation added
- [x] No breaking changes
- [x] Security considerations addressed
- [x] Error handling implemented
- [x] Code follows project patterns

## Notes

- Console logging is used for debugging (can be replaced with proper logging library if preferred)
- Some TODOs in code are outdated (password/key auth is fully implemented)
- Integration tests provide structure for future expansion with real test infrastructure
