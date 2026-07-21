// Jest config for the Expo app. jest-expo supplies the RN/Expo transform;
// tests mock native modules (expo-file-system, ExtensionStorage, ./api) so the
// pure queue/drain logic runs under Node without a device.
module.exports = {
  preset: "jest-expo",
  setupFiles: ["<rootDir>/jest.setup.js"],
  testMatch: ["**/*.test.ts"],
  transformIgnorePatterns: [
    "node_modules/(?!((jest-)?react-native|@react-native(-community)?|expo(nent)?|@expo(nent)?/.*|@expo-google-fonts/.*|@bacons/.*|react-navigation|@react-navigation/.*|@unimodules/.*|unimodules|sentry-expo|native-base|react-native-svg|supercluster|kdbush))",
  ],
};
