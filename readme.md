z-live
---
> 重构了 [live-go](https://github.com/gwuhaolin/livego) 部分代码  
> 代码方面会简洁一些 解决了一些bug 虽然注释还是很少  
> 目前支持rtmp推流时，http-flv、hls、本地实时保存功能  
> 里面还附带flv.js的简单demo  
> 支持webrtc sfu，dataChannel，视频保存，多人音视频通话  
> 
  
推流  
./ffmpeg -re -i demo.flv -c copy -f flv rtmp://127.0.0.1:1935/live/demo -loglevel debug  
  
  
http-flv  
浏览器打开 http://localhost:1937/httpFlv.html?u=%2Flive%2Fdemo.flv  
这个附带了flv.js的使用  

hls  
mac safari直接打开 http://localhost:1936/live/demo/demo.m3u8  

实时文件保存  
保存在项目目录下 默认.flv格式