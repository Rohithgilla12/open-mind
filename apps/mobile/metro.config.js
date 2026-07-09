// Learn more https://docs.expo.io/guides/customizing-metro
// withShareExtension teaches Metro to build the second (share-extension) bundle
// from index.share.tsx alongside the main app bundle — required by
// expo-share-extension.
const { getDefaultConfig } = require("expo/metro-config");
const { withShareExtension } = require("expo-share-extension/metro");

module.exports = withShareExtension(getDefaultConfig(__dirname), {
  isCSSEnabled: true,
});
