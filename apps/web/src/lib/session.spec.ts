import { api, tokenStore } from './api';
import { endSession } from './session';

jest.mock('./api', () => ({
  api: { logout: jest.fn() },
  tokenStore: { clear: jest.fn() },
}));

const mockedApi = api as unknown as { logout: jest.Mock };
const mockedStore = tokenStore as unknown as { clear: jest.Mock };

describe('endSession', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('revokes the session, clears the token and redirects to sign-in', async () => {
    mockedApi.logout.mockResolvedValue(undefined);
    const navigate = jest.fn();

    await endSession(navigate);

    expect(mockedApi.logout).toHaveBeenCalledTimes(1);
    expect(mockedStore.clear).toHaveBeenCalledTimes(1);
    expect(navigate).toHaveBeenCalledWith('/signin');
  });

  it('still clears the token and redirects when the server call fails', async () => {
    mockedApi.logout.mockRejectedValue(new Error('already expired'));
    const navigate = jest.fn();

    await endSession(navigate);

    expect(mockedStore.clear).toHaveBeenCalledTimes(1);
    expect(navigate).toHaveBeenCalledWith('/signin');
  });
});
