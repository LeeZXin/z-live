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

webrtc服务端  
dataChannel 打开 http://localhost:1939/data-channel.html  
音视频保存打开 http://localhost:1939/video.html  
多人视频通话打开 http://localhost:1939/room.html  

多人音视频通讯采用多peerConnection架构  
即有多少个成员，一个客户端就创建多少个peerConnection  
使得一个房间可在多个服务器之间传输数据   
可用于服务器调度和容灾考虑  
例如有三个成员 A、B、C  
A客户端会创建三个peerConnection  
1、一个用于将本地音视频传输到服务器，称media连接  
2、接收B的音视频数据，称B->A forward连接  
3、接收C的音视频数据，称C->A forward连接  
B、C同理各三个peerConnection, 一个media连接和两个forward连接  
A的media连接和B->A forward连接 C->A forward连接必须在同一个服务器  
同理  
B的media连接和A->B forward连接 C->B forward连接必须在同一个服务器  
C的media连接和A->C forward连接 B->C forward连接必须在同一个服务器  
但这三组连接可以不用在同一台服务器  
细品吧  


