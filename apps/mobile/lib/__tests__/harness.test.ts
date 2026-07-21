import AsyncStorage from "@react-native-async-storage/async-storage";

test("jest harness runs and AsyncStorage is mocked", async () => {
  await AsyncStorage.setItem("k", "v");
  expect(await AsyncStorage.getItem("k")).toBe("v");
});
