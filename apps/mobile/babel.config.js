// Reanimated 4 needs the worklets Babel plugin, and it must be listed last.
// Adding an explicit config replaces Expo's implicit default preset.
module.exports = function (api) {
  api.cache(true);
  return {
    presets: ["babel-preset-expo"],
    plugins: ["react-native-worklets/plugin"],
  };
};
