import 'package:flutter/material.dart';
import 'package:REPLACE_ME_DART_PACKAGE_NAME/src/bridge_generated.dart';
import 'package:REPLACE_ME_DART_PACKAGE_NAME/src/api/lib.dart';

Future<void> main() async {
  FlutterGoBridge.initialize();
  runApp(const MyApp());
}

class MyApp extends StatelessWidget {
  const MyApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      home: Scaffold(
        appBar: AppBar(title: const Text('flutter_go_bridge quickstart')),
        body: Center(
          child: Text(
            'Action: Call go `greet("Tom")`\nResult: `${greeting(name: "Tom")}`',
          ),
        ),
      ),
    );
  }
}
