export function MockAuthProvider({ children }) {
  const fakeUser = { username: "test", roles: ["admin"] };
  return (
    <AuthContext.Provider value={{
      isAuthenticated: true,
      isLoading: false,
      user: fakeUser,
      login: async () => {},
      logout: () => {},
    }}>
      {children}
    </AuthContext.Provider>
  );
}
