var box = new ReconnectingWebSocket(location.protocol.replace("http","ws") + "//" + location.host + "/ws/chat1");

box.onmessage = function(message) {
  var data = JSON.parse(message.data);
  $("#chat-text").append("<div class='panel panel-default'><div class='panel-heading'>" + $('<span/>').text(data.handle).html() + "</div><div class='panel-body'>" + $('<span/>').text(data.text).html() + "</div></div>");
  $("#chat-text").stop().animate({
    scrollTop: $('#chat-text')[0].scrollHeight
  }, 800);
};

box.onclose = function(){
    console.log('box closed');
    this.box = new WebSocket(box.url);

};

$("#input-form").on("submit", function(event) {
  event.preventDefault();
  var handle = $("#input-handle")[0].value;
  var text   = $("#input-text")[0].value;
  box.send(JSON.stringify({ handle: handle, text: text }));
  $("#input-text")[0].value = "";
});


var box2 = new ReconnectingWebSocket(location.protocol.replace("http","ws") + "//" + location.host + "/ws/chat2");
box2.onmessage = function(message) {
  var data = JSON.parse(message.data);
  $("#chat-text2").append("<div class='panel panel-default'><div class='panel-heading'>" + $('<span/>').text(data.handle).html() + "</div><div class='panel-body'>" + $('<span/>').text(data.text).html() + "</div></div>");
  $("#chat-text2").stop().animate({
    scrollTop: $('#chat-text2')[0].scrollHeight
  }, 800);
};

box2.onclose = function(){
    console.log('box2 closed');
    this.box2 = new WebSocket(box2.url);

};

$("#input-form2").on("submit", function(event) {
  event.preventDefault();
  var handle = $("#input-handle2")[0].value;
  var text   = $("#input-text2")[0].value;
  box2.send(JSON.stringify({ handle: handle, text: text }));
  $("#input-text2")[0].value = "";
});
