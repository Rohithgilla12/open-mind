// babel-preset-expo (SDK 50+) auto-injects the Reanimated worklets Babel plugin
// when react-native-worklets is installed — so it is intentionally NOT listed
// here; adding it explicitly double-applies the transform. Keep this file only
// to make the preset explicit and to document why the worklets plugin is absent.
module.exports = function (api) {
  api.cache(true);
  return {
    presets: ["babel-preset-expo"],
  };
};
